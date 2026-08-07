package mts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/CerealKiller97/preuzmi.me/pkg/dto"
	"github.com/CerealKiller97/preuzmi.me/pkg/repositories/receipts"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/provider"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/storage"
	"github.com/CerealKiller97/preuzmi.me/pkg/utils"
	"github.com/rs/zerolog"
)

const (
	tokenURL     = "https://moj.mts.rs/selfcare/b2c/auth/token"
	loginURL     = "https://moj.mts.rs/selfcare/b2c/user/authorize"
	receiptsURL  = "https://moj.mts.rs/hybris/selfcare/b2c/v1/user/billgroups/?includeBills=true"
	exportPDFURL = "https://moj.mts.rs/hybris/selfcare/b2c/v1/user/bills/export?invoiceNumber=%s&billingAccountId=%s"
)

var _ provider.Interface = &Service{}

type (
	loginRequest struct {
		Username   string `json:"userId"`
		Password   string `json:"encodedPassword"`
		RememberMe bool   `json:"rememberMe"`
	}

	accessTokenResponse struct {
		Token string `json:"token"`
	}

	Service struct {
		logger   zerolog.Logger
		storage  storage.Interface
		receipts *receipts.Repository
		http     *http.Client
		config   config.Credentials
		// name is the account key: "mts" for the primary account, or "mts-<slug>"
		// for an extra one. It is the receipt's base filename and its provider
		// column in the receipts index, so each account files separately.
		name string
	}
)

func New(
	name string,
	config config.Credentials,
	storage storage.Interface,
	logger zerolog.Logger,
	receiptsStore *receipts.Repository,
) *Service {
	return &Service{
		name:   name,
		config: config,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
		storage:  storage,
		receipts: receiptsStore,
		logger:   logger,
	}
}

func (s *Service) DownloadReceipt() error {
	cookie, err := s.login()
	if err != nil {
		s.logger.Err(err).Msg("Error while logging in")
		return err
	}

	token, err := s.getAccessToken(cookie)
	if err != nil {
		s.logger.Error().Err(err).Msg("Error getting access token")
		return err
	}

	bills, err := s.getReceipts(token)
	if err != nil {
		s.logger.Error().Err(err).Msg("Error getting receipts")
		return err
	}

	s.logger.Info().Int("billsCount", len(bills)).Msg("Successfully fetched receipts")

	if len(bills) == 0 {
		return fmt.Errorf("mts returned no bills")
	}

	// The bills are not returned in any guaranteed order, so pick the most
	// recent one by its own month/year rather than trusting the array order.
	bill := latestBill(bills)

	// The bill carries its own month/year, which is the authoritative period for
	// the receipt. Fall back to the previous-month heuristic only if the API
	// omits them.
	period := billPeriod(bill)

	if err := s.downloadReceipt(bill.InvoiceNumber, bill.BillingAccountID, token, period); err != nil {
		s.logger.Err(err).
			Str("invoiceNumber", bill.InvoiceNumber).
			Str("billingAccountId", bill.BillingAccountID).
			Msg("Error downloading receipt")

		return err
	}

	// Record the receipt's price and paid status. Best-effort: the PDF is already
	// saved, so a database hiccup must not fail the download.
	if s.receipts != nil {
		ctx := context.Background()
		if err := s.receipts.SetPrice(ctx, s.name, period, billPrice(bill)); err != nil {
			s.logger.Err(err).Str("period", period).Msg("Failed to record MTS receipt price")
		}
		if err := s.receipts.SetStatus(ctx, s.name, period, billStatus(bill)); err != nil {
			s.logger.Err(err).Str("period", period).Msg("Failed to record MTS receipt status")
		}
	}

	return nil
}

// billStatus maps the MTS paid state onto the label stored in the receipts
// table ("plaćeno" / "neplaćeno").
func billStatus(bill dto.Bill) string {
	if bill.Status == dto.Paid {
		return receipts.StatusPaid
	}

	return receipts.StatusUnpaid
}

// latestBill returns the most recent bill by (year, month). The caller must
// pass a non-empty slice.
func latestBill(bills []dto.Bill) dto.Bill {
	latest := bills[0]
	for _, b := range bills[1:] {
		if b.Year > latest.Year || (b.Year == latest.Year && b.Month > latest.Month) {
			latest = b
		}
	}

	return latest
}

// billPeriod returns the "MM-YYYY" folder for a bill, preferring the bill's own
// month/year and falling back to the previous-month heuristic when the API does
// not provide them.
//
// MTS reports month 0-indexed (0 = January … 11 = December), so the July bill
// arrives as month 6. Add one to get the human 1-12 month used in the folder;
// without this the receipt is filed one month early (July under 06-YYYY).
func billPeriod(bill dto.Bill) string {
	if bill.Month >= 0 && bill.Month <= 11 && bill.Year > 0 {
		return fmt.Sprintf("%02d-%d", bill.Month+1, bill.Year)
	}

	return utils.PreviousMonthFolder()
}

// billPrice returns the receipt amount. It prefers the numeric totalAmount when
// the response includes it, and otherwise parses the formatted string, which is
// the only amount the raw API response carries.
func billPrice(bill dto.Bill) float64 {
	if bill.TotalAmount != 0 {
		return bill.TotalAmount
	}

	return parsePrice(bill.TotalAmountFormatted)
}

// parsePrice converts an MTS formatted amount like "1.819,46" (Serbian locale:
// "." groups thousands, "," is the decimal separator, plus an optional currency
// suffix) into a float. An unparseable value yields 0.
func parsePrice(formatted string) float64 {
	var b strings.Builder
	for _, r := range formatted {
		if (r >= '0' && r <= '9') || r == '.' || r == ',' || r == '-' {
			b.WriteRune(r)
		}
	}

	s := strings.ReplaceAll(b.String(), ".", "") // drop thousands separators
	s = strings.ReplaceAll(s, ",", ".")          // decimal comma -> dot

	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}

	return f
}

func (s *Service) login() (string, error) {
	login := loginRequest{
		Username:   s.config.Username,
		Password:   s.base64EncodePassword(s.config.Password),
		RememberMe: false,
	}

	json, err := json.Marshal(login)
	if err != nil {
		s.logger.Err(err).Msg("Error marshalling login request")
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, loginURL, bytes.NewBuffer(json))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.http.Do(req)
	if err != nil {
		s.logger.Err(err).Msg("Error making login request")
		return "", err
	}

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("making login request failed")
	}
	defer resp.Body.Close()

	return resp.Header.Get("Set-Cookie"), nil
}

func (s *Service) base64EncodePassword(password string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(password))

	return "<b64>" + encoded + "</b64>"
}

func (s *Service) getAccessToken(cookie string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, tokenURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Cookie", cookie)

	resp, err := s.http.Do(req)
	if err != nil {
		s.logger.Err(err).Msg("Error making access token request")
		return "", err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("getting access token failed: %s", resp.Status)
	}

	accessToken, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var accessTokenResponse accessTokenResponse
	if err = json.Unmarshal(accessToken, &accessTokenResponse); err != nil {
		return "", err
	}

	return accessTokenResponse.Token, nil
}

func (s *Service) getReceipts(token string) ([]dto.Bill, error) {
	// Implement logic to fetch receipts
	req, err := http.NewRequest(http.MethodGet, receiptsURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("getting receipts failed: %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	response := dto.GetReceiptsResponse{}

	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}

	if len(response.BillGroups) == 0 {
		return nil, fmt.Errorf("mts returned no bill groups")
	}

	return response.BillGroups[0].Bills, nil
}

func (s *Service) downloadReceipt(invoiceNumber string, billingAccountId string, token string, period string) error {
	url := fmt.Sprintf(exportPDFURL, invoiceNumber, billingAccountId)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("getting receipt failed: %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("%s/%s.pdf", period, s.name)

	if err := s.storage.Save(context.Background(), key, data); err != nil {
		return err
	}

	s.logger.Info().
		Str("key", key).
		Msg("Successfully downloaded receipt")

	return nil
}
