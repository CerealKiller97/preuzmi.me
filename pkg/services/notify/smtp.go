package notify

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/CerealKiller97/preuzmi.me/pkg/config"
)

var _ Sender = &SMTPSender{}

// SMTPSender delivers notifications over SMTP.
type SMTPSender struct {
	cfg config.SMTP
}

func NewSMTP(cfg config.SMTP) (*SMTPSender, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("smtp host is empty")
	}
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	if cfg.From == "" || cfg.To == "" {
		return nil, fmt.Errorf("smtp from and to are required")
	}

	return &SMTPSender{cfg: cfg}, nil
}

func (s *SMTPSender) Send(subject, body string) error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	msg := buildMessage(s.cfg.From, s.cfg.To, subject, body)
	recipients := []string{s.cfg.To}

	var auth smtp.Auth
	if s.cfg.Username != "" {
		auth = smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	}

	// Port 465 is implicit TLS (SMTPS). Everything else gets STARTTLS when
	// the server advertises it, which covers the usual 587 submission path.
	if s.cfg.Port == 465 {
		return sendSMTPS(addr, s.cfg.Host, auth, s.cfg.From, recipients, msg)
	}

	return smtp.SendMail(addr, auth, s.cfg.From, recipients, msg)
}

func buildMessage(from, to, subject, body string) []byte {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteString("\r\n")
	}

	return []byte(b.String())
}

func sendSMTPS(addr, host string, auth smtp.Auth, from string, to []string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host})
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}

	if err := client.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return err
		}
	}

	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}

	return client.Quit()
}
