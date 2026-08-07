package eps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
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
	authenticateURL        = "https://apiportal.eps.rs/api/account/authenticate"
	commercialContractsURL = "https://apiportal.eps.rs/api/contracts/commercial-contracts"
	financialDocumentsURL  = "https://apiportal.eps.rs/api/finances/financial-documents/search"
	paymentsURL            = "https://apiportal.eps.rs/api/finances/payments/search"
	pdfURL                 = "https://apiportal.eps.rs/api/finances/financial-documents/pdf?Id=%s"

	// deviceTypeID identifies the client kind to the EPS API; 1 is the web
	// portal value observed from apiportal.eps.rs.
	deviceTypeID = 1

	// receiptsPageSize mirrors the portal's default page size for the
	// financial-documents and payments grids.
	receiptsPageSize = 12

	// epsDateLayout is the timestamp format EPS uses for issueDate/paymentDate.
	epsDateLayout = "2006-01-02T15:04:05"

	// amountEpsilon absorbs float rounding when matching a payment to a bill.
	amountEpsilon = 0.005
)

var _ provider.Interface = &Service{}

type (
	Service struct {
		logger   zerolog.Logger
		storage  storage.Interface
		receipts *receipts.Repository
		http     *http.Client
		config   config.Credentials
		// name is the account key: "eps" for the primary account, or "eps-<slug>"
		// for an extra one. It is the receipt's base filename and its provider
		// column in the receipts index, so each account files separately.
		name string
	}

	authenticateRequest struct {
		Username     string `json:"username"`
		Password     string `json:"password"`
		DeviceTypeID int    `json:"deviceTypeId"`
	}

	authenticateResponse struct {
		ErrorMessage string `json:"errorMessage"`
		Code         int    `json:"code"`
		Success      bool   `json:"success"`
		Token        struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
		} `json:"token"`
	}

	// commercialContract is one entry of the /contracts/commercial-contracts
	// array. ID is kept raw so it decodes whether EPS returns it as a JSON
	// number or a quoted string, and round-trips back into request bodies with
	// its original type.
	commercialContract struct {
		ID json.RawMessage `json:"id"`
	}

	// financialDocumentsSearchRequest is the DevExtreme-style search payload for
	// the financial-documents grid. The nil pointers marshal to JSON null.
	financialDocumentsSearchRequest struct {
		Take                    int             `json:"take"`
		SortColumnName          string          `json:"sortColumnName"`
		SortColumnDirection     string          `json:"sortColumnDirection"`
		FinancialDocumentTypeID *int            `json:"financialDocumentTypeId"`
		Month                   *int            `json:"month"`
		Year                    *int            `json:"year"`
		CommercialContractID    json.RawMessage `json:"commercialContractId"`
	}

	// financialDocument is one entry of the financial-documents grid. ID is kept
	// raw so it round-trips into the pdf query param with its original JSON type.
	// IssueDate is used to decide whether a later payment settled this bill.
	financialDocument struct {
		ID        json.RawMessage `json:"id"`
		IssueDate string          `json:"issueDate"`
		Amount    float64         `json:"amount"`
		Month     int             `json:"month"`
		Year      int             `json:"year"`
	}

	// financialDocumentsSearchResponse is the DevExtreme grid envelope.
	financialDocumentsSearchResponse struct {
		Data       []financialDocument `json:"data"`
		TotalCount int                 `json:"totalCount"`
	}

	// paymentsSearchRequest is the DevExtreme-style search payload for the
	// payments grid. Filters is always an empty object; the nil pointers marshal
	// to JSON null.
	paymentsSearchRequest struct {
		Take                 int             `json:"take"`
		SortColumnName       string          `json:"sortColumnName"`
		SortColumnDirection  string          `json:"sortColumnDirection"`
		Filters              struct{}        `json:"filters"`
		CommercialContractID json.RawMessage `json:"commercialContractId"`
		AmountFrom           *float64        `json:"amountFrom"`
		AmountTo             *float64        `json:"amountTo"`
	}

	// payment is one entry of the payments grid.
	payment struct {
		PaymentDate string  `json:"paymentDate"`
		Amount      float64 `json:"amount"`
		IsRefund    bool    `json:"isRefund"`
	}

	// paymentsSearchResponse is the DevExtreme grid envelope for payments.
	paymentsSearchResponse struct {
		Data []payment `json:"data"`
	}
)

func New(
	name string,
	config config.Credentials,
	logger zerolog.Logger,
	storage storage.Interface,
	receiptsStore *receipts.Repository,
) *Service {
	return &Service{
		name:     name,
		config:   config,
		logger:   logger,
		storage:  storage,
		receipts: receiptsStore,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (s *Service) DownloadReceipt() error {
	token, err := s.authenticate()
	if err != nil {
		s.logger.Err(err).Msg("Error while authenticating")
		return err
	}

	s.logger.Info().Msg("Successfully authenticated with EPS")

	contractID, err := s.getContractID(token)
	if err != nil {
		s.logger.Err(err).Msg("Error while fetching commercial contract")
		return err
	}

	s.logger.Info().
		Str("contractId", strings.Trim(string(contractID), `"`)).
		Msg("Resolved EPS commercial contract")

	documents, err := s.getReceipts(token, contractID)
	if err != nil {
		s.logger.Err(err).Msg("Error while fetching receipts")
		return err
	}

	s.logger.Info().Int("receiptsCount", len(documents)).Msg("Fetched EPS financial documents")

	if len(documents) == 0 {
		return fmt.Errorf("eps returned no financial documents")
	}

	// Documents come back newest-first, so the first one is the latest receipt.
	latest := documents[0]
	receiptID := strings.Trim(string(latest.ID), `"`)
	if receiptID == "" {
		return fmt.Errorf("eps financial document has empty id")
	}

	// File under the bill's own month/year so the folder is stable across
	// re-downloads. That stability is what lets a receipt marked paid today be
	// verified on a later run: both writes key off the same (provider, period).
	period := billPeriod(latest)

	if err := s.downloadReceipt(token, receiptID, period); err != nil {
		s.logger.Err(err).Str("receiptId", receiptID).Msg("Error downloading receipt")
		return err
	}

	// Record the receipt's price and paid status. Best-effort: the PDF is already
	// saved, so a database hiccup must not fail the download.
	if s.receipts != nil {
		ctx := context.Background()

		if err := s.receipts.SetPrice(ctx, s.name, period, latest.Amount); err != nil {
			s.logger.Err(err).Str("period", period).Msg("Failed to record EPS receipt price")
		}

		// EPS has no status field, so derive paid state from the payments grid:
		// a bill is paid once a matching payment exists. Only touch the stored
		// status when the payments fetch actually succeeds.
		payments, err := s.getPayments(token, contractID)
		if err != nil {
			s.logger.Err(err).Msg("Error fetching EPS payments; leaving status unchanged")
		} else {
			status := receipts.StatusUnpaid
			if billSettled(latest, payments) {
				status = receipts.StatusPaid
			}
			if err := s.receipts.SetStatus(ctx, s.name, period, status); err != nil {
				s.logger.Err(err).Str("period", period).Msg("Failed to record EPS receipt status")
			}
		}
	}

	return nil
}

// billPeriod returns the "MM-YYYY" folder for a bill, preferring the bill's own
// month/year and falling back to the previous-month heuristic when the API does
// not provide them.
func billPeriod(doc financialDocument) string {
	if doc.Month >= 1 && doc.Month <= 12 && doc.Year > 0 {
		return fmt.Sprintf("%02d-%d", doc.Month, doc.Year)
	}

	return utils.PreviousMonthFolder()
}

// billSettled reports whether the bill has been paid: a non-refund payment of
// the same amount whose paymentDate falls after the bill's issueDate.
func billSettled(bill financialDocument, payments []payment) bool {
	issued, err := time.Parse(epsDateLayout, bill.IssueDate)
	if err != nil {
		return false
	}

	for _, p := range payments {
		if p.IsRefund {
			continue
		}
		if math.Abs(p.Amount-bill.Amount) > amountEpsilon {
			continue
		}

		paid, err := time.Parse(epsDateLayout, p.PaymentDate)
		if err != nil {
			continue
		}
		if paid.After(issued) {
			return true
		}
	}

	return false
}

// downloadReceipt fetches the PDF for the given financial-document id and
// persists it under the given period folder.
func (s *Service) downloadReceipt(token, receiptID, period string) error {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf(pdfURL, receiptID), nil)
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
		return fmt.Errorf("eps pdf download failed: %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("%s/%s.pdf", period, s.name)

	if err := s.storage.Save(context.Background(), key, data); err != nil {
		return err
	}

	s.logger.Info().Str("key", key).Msg("Successfully downloaded receipt")

	return nil
}

// getContractID fetches the account's commercial contracts and returns the raw
// JSON id of the first one, which later calls use to scope bill and receipt
// requests. The value is kept raw so it round-trips into request bodies with
// its original JSON type (number or string).
func (s *Service) getContractID(token string) (json.RawMessage, error) {
	req, err := http.NewRequest(http.MethodGet, commercialContractsURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.http.Do(req)
	if err != nil {
		s.logger.Err(err).Msg("Error making commercial-contracts request")
		return nil, err
	}

	defer resp.Body.Close() //nolint:errcheck // best-effort body drain

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("eps commercial-contracts failed: %s", resp.Status)
	}

	var contracts []commercialContract
	if err := json.Unmarshal(body, &contracts); err != nil {
		return nil, err
	}

	if len(contracts) == 0 {
		return nil, fmt.Errorf("eps returned no commercial contracts")
	}

	id := contracts[0].ID
	if len(id) == 0 || strings.Trim(string(id), `"`) == "" {
		return nil, fmt.Errorf("eps commercial contract has empty id")
	}

	return id, nil
}

// getReceipts searches the financial-documents grid for the given commercial
// contract and returns the document entries, newest first.
func (s *Service) getReceipts(token string, contractID json.RawMessage) ([]financialDocument, error) {
	payload := financialDocumentsSearchRequest{
		Take:                 receiptsPageSize,
		SortColumnName:       "issueDate",
		SortColumnDirection:  "DESC",
		CommercialContractID: contractID,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, financialDocumentsURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.http.Do(req)
	if err != nil {
		s.logger.Err(err).Msg("Error making financial-documents request")
		return nil, err
	}

	defer resp.Body.Close() //nolint:errcheck // best-effort body drain

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("eps financial-documents failed: %s", resp.Status)
	}

	var res financialDocumentsSearchResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}

	return res.Data, nil
}

// getPayments searches the payments grid for the given commercial contract and
// returns the payment entries, newest first.
func (s *Service) getPayments(token string, contractID json.RawMessage) ([]payment, error) {
	payload := paymentsSearchRequest{
		Take:                 receiptsPageSize,
		SortColumnName:       "paymentDate",
		SortColumnDirection:  "DESC",
		CommercialContractID: contractID,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, paymentsURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.http.Do(req)
	if err != nil {
		s.logger.Err(err).Msg("Error making payments request")
		return nil, err
	}

	defer resp.Body.Close() //nolint:errcheck // best-effort body drain

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("eps payments failed: %s", resp.Status)
	}

	var res paymentsSearchResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}

	return res.Data, nil
}

// authenticate exchanges the configured username/password for an EPS access
// token via the apiportal.eps.rs authenticate endpoint.
func (s *Service) authenticate() (string, error) {
	payload := authenticateRequest{
		Username:     s.config.Username,
		Password:     s.config.Password,
		DeviceTypeID: deviceTypeID,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		s.logger.Err(err).Msg("Error marshalling authenticate request")
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, authenticateURL, bytes.NewReader(data))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.http.Do(req)
	if err != nil {
		s.logger.Err(err).Msg("Error making authenticate request")
		return "", err
	}

	defer resp.Body.Close() //nolint:errcheck // best-effort body drain

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("eps authenticate failed: %s", resp.Status)
	}

	var res authenticateResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return "", err
	}

	if !res.Success || res.Token.AccessToken == "" {
		return "", fmt.Errorf("eps authenticate rejected: %s", res.ErrorMessage)
	}

	return res.Token.AccessToken, nil
}
