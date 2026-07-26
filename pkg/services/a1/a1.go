package a1

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/CerealKiller97/preuzmi.me/pkg/repositories/receipts"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/provider"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/storage"
	"github.com/CerealKiller97/preuzmi.me/pkg/utils"
	"github.com/rs/zerolog"
)

const (
	loginPageURL       = "https://a1.rs/moj_a1"
	loginURL           = "https://asmp.a1.rs/asmp/ProcessLoginServlet"
	profileURL         = "https://a1.rs/webscapi/v1/profile?include=customer,subscriptions"
	subscriptionURLFmt = "https://a1.rs/webscapi/v1/subscriptions/%s?include=units,bills,addons"
	downloadBillURL    = "https://a1.rs/webscapi/v1/download-bill/%s"

	// a1DateLayout is the RFC3339 timestamp format A1 uses for bill dates.
	a1DateLayout = "2006-01-02T15:04:05-07:00"

	fileName = "a1"

	// userAgent makes the requests look like a normal browser; some ASMP
	// front-ends reject requests without one.
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"

	// apiReferer and apiAuthorization are the fixed webscapi headers the a1.rs
	// front-end sends; the Authorization is a static app-level basic credential.
	apiReferer       = "https://a1.rs/moja1"
	apiAuthorization = "Basic b25lYXBwdW5kYWJvdEBiMmIudmlwbW9iaWxlLnJzOlJtcFd2a1JiQklYX0JiaHhpejRxYVVyY0tJNjhmQlFQZURrZWpWMWNqMjA="
)

// as_sfid / as_fid are Eloqua tracking tokens injected into the login form per
// page load. They are scraped from the login page rather than hardcoded.
var (
	sfidRe = regexp.MustCompile(`name="as_sfid"\s+value="([^"]*)"`)
	fidRe  = regexp.MustCompile(`name="as_fid"\s+value="([^"]*)"`)
)

type (
	Service struct {
		logger   zerolog.Logger
		storage  storage.Interface
		receipts *receipts.Repository
		http     *http.Client
		config   config.Credentials
	}

	// profileResponse is the JSON:API payload of /webscapi/v1/profile; only the
	// subscription and customer relationship ids are needed.
	profileResponse struct {
		Data struct {
			Relationships struct {
				Subscriptions struct {
					Data []struct {
						ID string `json:"id"`
					} `json:"data"`
				} `json:"subscriptions"`
				Customer struct {
					Data struct {
						ID string `json:"id"`
					} `json:"data"`
				} `json:"customer"`
			} `json:"relationships"`
		} `json:"data"`
	}

	// subscriptionResponse is the JSON:API payload of /webscapi/v1/subscriptions;
	// the bills live in the "included" array as resources of type "bill".
	subscriptionResponse struct {
		Included []includedResource `json:"included"`
	}

	includedResource struct {
		Type       string     `json:"type"`
		ID         string     `json:"id"`
		Attributes billFields `json:"attributes"`
	}

	// billFields are the bill attributes used to download and record a receipt.
	billFields struct {
		TotalAmount             float64 `json:"totalAmount"`
		PaymentStatus           string  `json:"paymentStatus"`
		StartDate               string  `json:"startDate"`
		EndDate                 string  `json:"endDate"`
		IsBillDownloadAvailable bool    `json:"isBillDownloadAvailable"`
	}

	// bill flattens an included bill resource to its id plus attributes.
	bill struct {
		ID     string
		Fields billFields
	}

	// billPdfResponse is the JSON:API payload of /webscapi/v1/download-bill; the
	// PDF bytes are base64-encoded in the content attribute.
	billPdfResponse struct {
		Data struct {
			Attributes struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
				Content string `json:"content"`
			} `json:"attributes"`
		} `json:"data"`
	}
)

var _ provider.Interface = &Service{}

func New(config config.Credentials, logger zerolog.Logger, storage storage.Interface, receiptsStore *receipts.Repository) *Service {
	// A cookie jar carries the session cookie the login sets (and any cookies
	// picked up along redirects) into the follow-up authenticated requests.
	jar, _ := cookiejar.New(nil) //nolint:errcheck // cookiejar.New never errors with a nil options

	return &Service{
		config:   config,
		logger:   logger,
		storage:  storage,
		receipts: receiptsStore,
		http: &http.Client{
			Timeout: 30 * time.Second,
			Jar:     jar,
		},
	}
}

func (s Service) DownloadReceipt() error {
	cookie, err := s.login()
	if err != nil {
		return err
	}

	subscriptionID, customerID, err := s.getProfile(cookie)
	if err != nil {
		s.logger.Err(err).Msg("Error while fetching A1 profile")
		return err
	}

	s.logger.Info().
		Str("subscriptionId", subscriptionID).
		Str("customerId", customerID).
		Msg("Resolved A1 subscription and customer")

	bills, err := s.getReceipts(subscriptionID, cookie)
	if err != nil {
		s.logger.Err(err).Msg("Error while fetching A1 receipts")
		return err
	}

	if len(bills) == 0 {
		return fmt.Errorf("a1 returned no bills")
	}

	latest := bills[0]
	period := billPeriod(latest)

	s.logger.Info().
		Int("billsCount", len(bills)).
		Str("billId", latest.ID).
		Float64("amount", latest.Fields.TotalAmount).
		Str("status", latest.Fields.PaymentStatus).
		Str("period", period).
		Msg("Fetched A1 bills")

	if err := s.downloadReceipt(latest.ID, subscriptionID, cookie, period); err != nil {
		s.logger.Err(err).Str("billId", latest.ID).Msg("Error downloading A1 receipt")
		return err
	}

	// Record price and paid status. Best-effort: the PDF is already saved, so a
	// database hiccup must not fail the download.
	if s.receipts != nil {
		ctx := context.Background()
		if err := s.receipts.SetPrice(ctx, fileName, period, latest.Fields.TotalAmount); err != nil {
			s.logger.Err(err).Str("period", period).Msg("Failed to record A1 receipt price")
		}
		if err := s.receipts.SetStatus(ctx, fileName, period, billStatus(latest)); err != nil {
			s.logger.Err(err).Str("period", period).Msg("Failed to record A1 receipt status")
		}
	}

	_ = customerID

	return nil
}

// downloadReceipt fetches the bill PDF (base64 inside a JSON:API envelope),
// decodes it, and persists it under the period folder.
func (s Service) downloadReceipt(billID, subscriptionID, cookie, period string) error {
	q := url.Values{}
	q.Set("filter[subscription-id]", subscriptionID)
	endpoint := fmt.Sprintf(downloadBillURL, billID) + "?" + q.Encode()

	req, err := s.newAPIRequest(http.MethodGet, endpoint, cookie)
	if err != nil {
		return err
	}

	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close() //nolint:errcheck // best-effort body drain

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("a1 bill download failed: %s", resp.Status)
	}

	var res billPdfResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return err
	}

	if !res.Data.Attributes.Success || res.Data.Attributes.Content == "" {
		return fmt.Errorf("a1 bill download rejected: %s", res.Data.Attributes.Message)
	}

	data, err := base64.StdEncoding.DecodeString(res.Data.Attributes.Content)
	if err != nil {
		return fmt.Errorf("a1 bill content is not valid base64: %w", err)
	}

	key := fmt.Sprintf("%s/%s.pdf", period, fileName)

	if err := s.storage.Save(context.Background(), key, data); err != nil {
		return err
	}

	s.logger.Info().Str("key", key).Msg("Successfully downloaded receipt")

	return nil
}

// billPeriod returns the "MM-YYYY" folder for a bill from its billing-period
// start date, falling back to the previous-month heuristic when it can't parse.
func billPeriod(b bill) string {
	if t, err := time.Parse(a1DateLayout, b.Fields.StartDate); err == nil {
		return fmt.Sprintf("%02d-%d", t.Month(), t.Year())
	}

	return utils.PreviousMonthFolder()
}

// billStatus maps A1's paymentStatus onto the receipts table label.
func billStatus(b bill) string {
	if strings.EqualFold(b.Fields.PaymentStatus, "paid") {
		return receipts.StatusPaid
	}

	return receipts.StatusUnpaid
}

// getProfile reads the logged-in profile and returns the default subscription
// id and the customer id. The session cookies from login are sent explicitly so
// the asmp.a1.rs SSO cookie reaches the a1.rs API (the jar also adds the a1.rs
// cookies for this host).
func (s Service) getProfile(cookie string) (subscriptionID, customerID string, err error) {
	req, err := s.newAPIRequest(http.MethodGet, profileURL, cookie)
	if err != nil {
		return "", "", err
	}

	resp, err := s.http.Do(req)
	if err != nil {
		return "", "", err
	}

	defer resp.Body.Close() //nolint:errcheck // best-effort body drain

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("a1 profile failed: %s", resp.Status)
	}

	var res profileResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return "", "", err
	}

	if len(res.Data.Relationships.Subscriptions.Data) == 0 {
		return "", "", fmt.Errorf("a1 profile has no subscriptions")
	}

	subscriptionID = res.Data.Relationships.Subscriptions.Data[0].ID
	customerID = res.Data.Relationships.Customer.Data.ID

	return subscriptionID, customerID, nil
}

// newAPIRequest builds a webscapi request carrying the fixed front-end headers
// and the forwarded session cookies.
func (s Service) newAPIRequest(method, rawURL, cookie string) (*http.Request, error) {
	req, err := http.NewRequest(method, rawURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Dnt", "1")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", apiReferer)
	req.Header.Set("X-Client", "web")
	req.Header.Set("Authorization", apiAuthorization)
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}

	return req, nil
}

// getReceipts fetches the subscription's bills, returning them newest first.
func (s Service) getReceipts(subscriptionID, cookie string) ([]bill, error) {
	endpoint := fmt.Sprintf(subscriptionURLFmt, subscriptionID)

	req, err := s.newAPIRequest(http.MethodGet, endpoint, cookie)
	if err != nil {
		return nil, err
	}

	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close() //nolint:errcheck // best-effort body drain

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("a1 subscription failed: %s", resp.Status)
	}

	var res subscriptionResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}

	bills := make([]bill, 0, len(res.Included))
	for _, r := range res.Included {
		if r.Type != "bill" {
			continue
		}
		bills = append(bills, bill{ID: r.ID, Fields: r.Attributes})
	}

	// Newest first, by billing-period start date.
	sort.Slice(bills, func(i, j int) bool {
		return bills[i].Fields.StartDate > bills[j].Fields.StartDate
	})

	return bills, nil
}

// login authenticates and returns a Cookie header carrying the session cookies
// from both A1 hosts, so callers can present them explicitly on the a1.rs API
// (the jar alone would not send the asmp.a1.rs SSO cookie cross-subdomain).
func (s Service) login() (string, error) {
	// 1. Load the login page: this both seeds the cookie jar and gives us the
	//    current Eloqua tokens. A failure here is not fatal — try the login
	//    without them, since ASMP often accepts the POST anyway.
	sfid, fid, err := s.loginTokens()
	if err != nil {
		s.logger.Warn().Err(err).Msg("Could not read A1 login tokens; attempting login without them")
	}

	// 2. Build the form body with the real field names from the HTML form.
	form := url.Values{}
	form.Set("userRequestURL", loginPageURL)
	form.Set("service", "LoginLevel10")
	form.Set("level", "30")
	form.Set("SetMsisdn", "false")
	form.Set("UserID", s.config.Username)
	form.Set("Password", s.config.Password)
	if sfid != "" {
		form.Set("as_sfid", sfid)
	}
	if fid != "" {
		form.Set("as_fid", fid)
	}

	req, err := http.NewRequest(http.MethodPost, loginURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}

	// These are HTTP headers, not body fields.
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Origin", "https://a1.rs")
	req.Header.Set("Referer", loginPageURL)

	resp, err := s.http.Do(req)
	if err != nil {
		s.logger.Err(err).Msg("Error making A1 login request")
		return "", err
	}

	defer resp.Body.Close() //nolint:errcheck // best-effort body drain

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	cookie := s.sessionCookies()

	// The exact success signal is not known yet — log enough to identify it
	// (status, final URL after redirects, cookies gathered, body size), then
	// assert on it here once confirmed.
	s.logger.Info().
		Int("status", resp.StatusCode).
		Str("finalURL", resp.Request.URL.String()).
		Int("cookies", len(s.http.Jar.Cookies(resp.Request.URL))).
		Int("bodyLen", len(body)).
		Msg("A1 login response")

	return cookie, nil
}

// sessionCookies collapses the cookies the jar holds for every A1 host into a
// single "name=value; ..." Cookie header, so the asmp.a1.rs SSO cookie can be
// forwarded to the a1.rs API alongside the a1.rs cookies.
func (s Service) sessionCookies() string {
	seen := map[string]struct{}{}
	var parts []string

	for _, raw := range []string{loginURL, loginPageURL, profileURL} {
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		for _, c := range s.http.Jar.Cookies(u) {
			if _, ok := seen[c.Name]; ok {
				continue
			}
			seen[c.Name] = struct{}{}
			parts = append(parts, c.Name+"="+c.Value)
		}
	}

	return strings.Join(parts, "; ")
}

// loginTokens fetches the login page and extracts the Eloqua form tokens.
func (s Service) loginTokens() (sfid, fid string, err error) {
	req, err := http.NewRequest(http.MethodGet, loginPageURL, nil)
	if err != nil {
		return "", "", err
	}

	req.Header.Set("User-Agent", userAgent)

	resp, err := s.http.Do(req)
	if err != nil {
		return "", "", err
	}

	defer resp.Body.Close() //nolint:errcheck // best-effort body drain

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	if m := sfidRe.FindSubmatch(body); m != nil {
		sfid = string(m[1])
	}
	if m := fidRe.FindSubmatch(body); m != nil {
		fid = string(m[1])
	}

	return sfid, fid, nil
}
