package esanduce

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/CerealKiller97/preuzmi.me/pkg/repositories/receipts"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/pdftext"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/provider"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/storage"
	"github.com/CerealKiller97/preuzmi.me/pkg/utils"
	"github.com/rs/zerolog"
)

const (
	LogTimeFormat = "2006-01-02 15:04:05"
	ApplicationID = "31C64209-93FC-490F-542B-08D6D6E198FE"
	RequestType   = "password"
	BaseURL       = "https://esanduceservice.infostan.rs/api"

	// identListURL lists the account's idents for issuer 1 (ЈКП Инфостан
	// Технологије); the ident identifies which space's bills to fetch.
	identListURL = BaseURL + "/SONUpit/identi_lista_izdavaoc/1"

	// statusDugaPaid is the Cyrillic value of status_duga that marks a bill as
	// paid; anything else counts as unpaid.
	statusDugaPaid = "плаћен"

	fileName = "esanduce"
)

type (
	Service struct {
		logger   zerolog.Logger
		storage  storage.Interface
		receipts *receipts.Repository
		http     *http.Client
		config   config.Credentials
	}

	LoginResponse struct {
		Token        string    `json:"token"`
		Expiration   time.Time `json:"expiration"`
		RefreshToken string    `json:"refresh_token"`
		Roles        string    `json:"roles"`
		Username     string    `json:"username"`
		Type         string    `json:"type"`
		OldType      any       `json:"old_type"`
		OldModel     any       `json:"old_model"`
		Language     string    `json:"language"`
	}

	Bill struct {
		TekuciRacun        string  `json:"tekuci_racun"`
		Model              string  `json:"model"`
		DatumValute        string  `json:"datum_valute"`
		MesecNaslov        string  `json:"mesecNaslov"`
		StatusDuga         string  `json:"status_duga"`
		IndVr              string  `json:"ind_vr"`
		Vrsta              string  `json:"vrsta"`
		DatumPopusta       string  `json:"datum_popusta"`
		Mesec              string  `json:"mesec"`
		ID                 string  `json:"id"`
		PozivNaBroj        string  `json:"poziv_na_broj"`
		Zaduzenje          float64 `json:"zaduzenje"`
		Ident              int     `json:"ident"`
		Naplata            float64 `json:"naplata"`
		Dug                float64 `json:"dug"`
		Ggmm               int     `json:"ggmm"`
		StatusDugaSifra    int     `json:"status_duga_sifra"`
		DozvoljenoPlacanje bool    `json:"dozvoljeno_placanje"`
		IndVazeci          bool    `json:"ind_vazeci"`
	}

	GetReceiptResponse struct {
		Summary    any    `json:"summary"`
		Data       []Bill `json:"data"`
		TotalCount int    `json:"totalCount"`
		GroupCount int    `json:"groupCount"`
	}

	IdentEntry struct {
		Ident         int    `json:"ident"`
		Vrsta         string `json:"vrsta"`
		IzdavaocNaziv string `json:"izdavaoc_naziv"`
		IzdavaocID    int    `json:"izdavaoc_id"`
		Tip           string `json:"tip"`
		Naselje       string `json:"naselje"`
	}

	IdentListResponse struct {
		Summary    any          `json:"summary"`
		Data       []IdentEntry `json:"data"`
		TotalCount int          `json:"totalCount"`
		GroupCount int          `json:"groupCount"`
	}

	GetUserInfoResponse struct {
		InterniPortal             any    `json:"interniPortal"`
		TelekomRacun              any    `json:"telekomRacun"`
		MaticniBroj               any    `json:"maticniBroj"`
		DobijanjeEmailObavestenja any    `json:"dobijanjeEmailObavestenja"`
		Pib                       any    `json:"pib"`
		PoslovnoIme               any    `json:"poslovnoIme"`
		DatumGledanjaTutorijala   string `json:"datumGledanjaTutorijala"`
		Email                     string `json:"email"`
		NazivPrikaz               string `json:"nazivPrikaz"`
		TipNaziv                  string `json:"tipNaziv"`
		Jmbg                      string `json:"jmbg"`
		BrojTelefona              string `json:"brojTelefona"`
		Ime                       string `json:"ime"`
		KorisnickoIme             string `json:"korisnickoIme"`
		Prezime                   string `json:"prezime"`
		TipID                     string `json:"tipId"`
		Jezik                     string `json:"jezik"`
		EmailVerifikovan          bool   `json:"emailVerifikovan"`
		OdgledanTutorijal         bool   `json:"odgledanTutorijal"`
		NeSaljiRacunPostom        bool   `json:"neSaljiRacunPostom"`
	}
)

var _ provider.Interface = &Service{}

func New(config config.Credentials, logger zerolog.Logger, storage storage.Interface, receiptsStore *receipts.Repository) *Service {
	return &Service{
		config:   config,
		logger:   logger,
		storage:  storage,
		receipts: receiptsStore,
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s Service) DownloadReceipt() error {
	accessToken, err := s.login()
	if err != nil {
		return err
	}

	ident, err := s.getIdent(accessToken)
	if err != nil {
		s.logger.Err(err).Msg("Error while fetching esanduce ident")
		return err
	}

	s.logger.Info().Int("ident", ident).Msg("Resolved esanduce ident")

	bills, err := s.getReceipts(accessToken, ident)
	if err != nil {
		s.logger.Err(err).Msg("Error while fetching esanduce receipts")
		return err
	}

	if len(bills) == 0 {
		return fmt.Errorf("esanduce returned no bills")
	}

	// Sorted by ggmm descending, so the first bill is the latest.
	bill := bills[0]
	period := periodFromGGMM(bill.Ggmm)

	if err := s.downloadReceipt(accessToken, ident, bill.Ggmm, period); err != nil {
		s.logger.Err(err).Int("ggmm", bill.Ggmm).Msg("Error downloading esanduce receipt")
		return err
	}

	// Record price and paid status. Best-effort: the PDF is already saved, so a
	// database hiccup must not fail the download.
	if s.receipts != nil {
		ctx := context.Background()
		if err := s.receipts.SetPrice(ctx, fileName, period, bill.Zaduzenje); err != nil {
			s.logger.Err(err).Str("period", period).Msg("Failed to record esanduce receipt price")
		}
		if err := s.receipts.SetStatus(ctx, fileName, period, billStatus(bill)); err != nil {
			s.logger.Err(err).Str("period", period).Msg("Failed to record esanduce receipt status")
		}
		if due, ok := parseDueDate(bill.DatumValute); ok {
			if err := s.receipts.SetDueAt(ctx, fileName, period, due.Unix()); err != nil {
				s.logger.Err(err).Str("period", period).Msg("Failed to record esanduce receipt due date")
			}
		}
	}

	return nil
}

// getReceipts fetches the ident's bills, newest first (sorted by ggmm desc).
func (s Service) getReceipts(token string, ident int) ([]Bill, error) {
	q := url.Values{}
	q.Set("skip", "0")
	q.Set("take", "10")
	q.Set("sort", `[{"selector":"ggmm","desc":true}]`)
	q.Set("filter", "")

	endpoint := fmt.Sprintf("%s/SONUpit/ident/%d/racuni?%s", BaseURL, ident, q.Encode())

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)

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
		return nil, fmt.Errorf("esanduce receipts failed: %s", resp.Status)
	}

	var res GetReceiptResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}

	return res.Data, nil
}

// downloadReceipt fetches the PDF for the given ident/ggmm and persists it under
// the period folder.
func (s Service) downloadReceipt(token string, ident, ggmm int, period string) error {
	endpoint := fmt.Sprintf("%s/SONUpit/ident/%d/zaduzenje/%d/stampa", BaseURL, ident, ggmm)

	req, err := http.NewRequest(http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close() //nolint:errcheck // best-effort body drain

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("esanduce pdf download failed: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// The stampa endpoint returns application/json: a JSON-encoded string
	// holding the base64 PDF, not raw PDF bytes. Unwrap the string, then decode.
	var encoded string
	if err := json.Unmarshal(body, &encoded); err != nil {
		return fmt.Errorf("esanduce pdf decode: unexpected response: %w", err)
	}

	if encoded == "" {
		return fmt.Errorf("esanduce returned an empty pdf for ggmm %d", ggmm)
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("esanduce pdf base64 decode: %w", err)
	}

	key := fmt.Sprintf("%s/%s.pdf", period, fileName)

	if err := s.storage.Save(context.Background(), key, data); err != nil {
		return err
	}

	s.logger.Info().Str("key", key).Msg("Successfully downloaded receipt")

	return nil
}

// periodFromGGMM converts esanduce's YYMM code (e.g. 2606) into the "MM-YYYY"
// folder, falling back to the previous-month heuristic on an out-of-range value.
func periodFromGGMM(ggmm int) string {
	year := 2000 + ggmm/100
	month := ggmm % 100

	if month < 1 || month > 12 {
		return utils.PreviousMonthFolder()
	}

	return fmt.Sprintf("%02d-%d", month, year)
}

// billStatus maps esanduce's status_duga onto the receipts table label
// ("plaćeno" / "neplaćeno").
func billStatus(bill Bill) string {
	if strings.TrimSpace(bill.StatusDuga) == statusDugaPaid {
		return receipts.StatusPaid
	}

	return receipts.StatusUnpaid
}

// parseDueDate turns the API's datum_valute into a local midnight time. The
// field arrives as either a Serbian "DD.MM.YYYY" string or an ISO date.
func parseDueDate(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	if t, ok := pdftext.ParseDate(raw); ok {
		return t, true
	}
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02",
	} {
		if t, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local), true
		}
	}

	return time.Time{}, false
}

// getIdent fetches the account's idents and returns the first one, which later
// calls use to scope the bill lookup.
func (s Service) getIdent(token string) (int, error) {
	req, err := http.NewRequest(http.MethodGet, identListURL, nil)
	if err != nil {
		return 0, err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.http.Do(req)
	if err != nil {
		return 0, err
	}

	defer resp.Body.Close() //nolint:errcheck // best-effort body drain

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("esanduce ident list failed: %s", resp.Status)
	}

	var res IdentListResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return 0, err
	}

	if len(res.Data) == 0 {
		return 0, fmt.Errorf("esanduce returned no idents")
	}

	return res.Data[0].Ident, nil
}

func (s Service) login() (string, error) {
	type LoginRequest struct {
		Username      string `json:"korisnickoIme"`
		Password      string `json:"lozinka"`
		ApplicationID string `json:"aplikacijaId"`
		Type          string `json:"tip"`
	}

	req := LoginRequest{
		Username:      s.config.Username,
		Password:      s.config.Password,
		ApplicationID: ApplicationID,
		Type:          RequestType,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	response, err := http.Post(
		BaseURL+"/korisnik/prijava",
		"application/json",
		bytes.NewReader(data),
	)
	if err != nil {
		return "", err
	}

	defer response.Body.Close() //nolint:errcheck // best-effort body drain

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}

	var res LoginResponse

	if err := json.Unmarshal(body, &res); err != nil {
		return "", err
	}

	return res.Token, nil
}
