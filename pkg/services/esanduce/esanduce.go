package esanduce

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/provider"
	"github.com/rs/zerolog"
	"io"
	"net/http"
	"time"
)

const (
	LogTimeFormat = "2006-01-02 15:04:05"
	ApplicationID = "31C64209-93FC-490F-542B-08D6D6E198FE"
	RequestType   = "password"
	BaseURL       = "https://esanduceservice.infostan.rs/api"
)

type (
	Service struct {
		config       config.Credentials
		logger       zerolog.Logger
		downloadPath string
		http         *http.Client
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

func New(config config.Credentials, logger zerolog.Logger, downloadPath string) *Service {
	return &Service{
		config:       config,
		logger:       logger,
		downloadPath: downloadPath,
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

	fmt.Println(accessToken)

	return nil
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

	defer func(Body io.ReadCloser) {
		if err := Body.Close(); err != nil {
		}
	}(response.Body)

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
