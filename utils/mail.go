package utils

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	SUCCESS = iota
	UNKNOWN_ERROR
	NO_CONFIG
	NO_HOST
	CANNOT_SEND_MAIL
	NO_RECIPIENT_SET
	NO_SENDER_SET
)

type Config struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`

	From string   `yaml:"from"`
	To   []string `yaml:"to"`
}

type Mail struct {
	cfg *Config

	errorMsg  string
	errorCode int
}

func Mailer(configPath string) *Mail {
	cfg := loadFromYaml(configPath)

	if cfg == nil {
		return &Mail{
			errorMsg:  "Failed to load configuration for mail module",
			errorCode: NO_CONFIG,
		}
	}

	m := &Mail{
		cfg:       cfg,
		errorCode: SUCCESS,
	}

	if m.cfg.Host == "" {
		m.errorMsg = "No mail host defined"
		m.errorCode = NO_HOST
	}

	return m
}

// Error returns last error message
func (m *Mail) Error() string {
	return m.errorMsg
}

// Send sends a mail
func (m *Mail) Send(message string) int {
	if len(m.cfg.To) == 0 {
		m.errorMsg = "No recipient set"
		return m.setError(NO_RECIPIENT_SET)
	}

	if m.cfg.From == "" {
		m.errorMsg = "No sender set"
		return m.setError(NO_SENDER_SET)
	}

	addr := fmt.Sprintf("%s:%d", m.cfg.Host, m.cfg.Port)

	conn, err := tls.Dial("tcp", addr, &tls.Config{
		ServerName: m.cfg.Host,
	})
	if err != nil {
		m.errorMsg = err.Error()
		return m.setError(CANNOT_SEND_MAIL)
	}
	defer conn.Close()

	host, _, _ := net.SplitHostPort(addr)
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		m.errorMsg = err.Error()
		return m.setError(CANNOT_SEND_MAIL)
	}
	defer client.Quit()

	if m.cfg.User != "" && m.cfg.Password != "" {
		auth := smtp.PlainAuth("", m.cfg.User, m.cfg.Password, m.cfg.Host)
		if err := client.Auth(auth); err != nil {
			m.errorMsg = err.Error()
			return m.setError(CANNOT_SEND_MAIL)
		}
	}

	if err := client.Mail(m.cfg.From); err != nil {
		m.errorMsg = err.Error()
		return m.setError(CANNOT_SEND_MAIL)
	}

	for _, rcpt := range m.cfg.To {
		if err := client.Rcpt(rcpt); err != nil {
			m.errorMsg = err.Error()
			return m.setError(CANNOT_SEND_MAIL)
		}
	}

	w, err := client.Data()
	if err != nil {
		m.errorMsg = err.Error()
		return m.setError(CANNOT_SEND_MAIL)
	}

	date := time.Now().Format("02 Jan 2006 15:04:05 -0700")

	headers := []string{
		fmt.Sprintf("From: %s", m.cfg.From),
		fmt.Sprintf("To: %s", strings.Join(m.cfg.To, ",")),
		"Content-Type: text/plain; charset=UTF-8",
		fmt.Sprintf("Date: %s", date),
		"",
	}

	body := strings.Join(headers, "\r\n") + message

	if _, err := w.Write([]byte(body)); err != nil {
		m.errorMsg = err.Error()
		return m.setError(CANNOT_SEND_MAIL)
	}

	if err := w.Close(); err != nil {
		m.errorMsg = err.Error()
		return m.setError(CANNOT_SEND_MAIL)
	}

	return SUCCESS
}

// ---------------- helpers ----------------

func (m *Mail) setError(code int) int {
	m.errorCode = code
	return code
}

func loadFromYaml(path string) *Config {

	var cfg Config

	yfile, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("Could not read config file: ", err)
		return nil
	}

	err = yaml.Unmarshal(yfile, &cfg)
	if err != nil {
		fmt.Println("Error loading configuration: ", err)
		return nil
	}
	return &cfg
}
