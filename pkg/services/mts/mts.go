package mts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/CerealKiller97/preuzmi.me/pkg/dto"
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

	fileName = "mts"
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
		config  config.Credentials
		storage storage.Interface
		logger  zerolog.Logger
		http    *http.Client
	}
)

func New(
	config config.Credentials,
	storage storage.Interface,
	logger zerolog.Logger,
) *Service {
	return &Service{
		config: config,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
		storage: storage,
		logger:  logger,
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

	invoiceNumber := bills[0].InvoiceNumber
	billingAccountId := bills[0].BillingAccountID

	if err := s.downloadReceipt(invoiceNumber, billingAccountId, token); err != nil {
		s.logger.Err(err).
			Str("invoiceNumber", invoiceNumber).
			Str("billingAccountId", billingAccountId).
			Str("token", token).
			Msg("Error downloading receipt")

		return err
	}

	return nil
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
		return "", fmt.Errorf("Error making login request")
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
		return "", fmt.Errorf("Error getting access token: %s", resp.Status)
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
		return nil, fmt.Errorf("Error getting receipts: %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	response := dto.GetReceiptsResponse{}

	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}

	return response.BillGroups[0].Bills, nil
}

func (s *Service) downloadReceipt(invoiceNumber string, billingAccountId string, token string) error {
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
		return fmt.Errorf("Error getting receipt: %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("%s/%s.pdf", utils.FormatFolderPath(), fileName)

	if err := s.storage.Save(context.Background(), key, data); err != nil {
		return err
	}

	s.logger.Info().
		Str("key", key).
		Msg("Successfully downloaded receipt")

	return nil
}
