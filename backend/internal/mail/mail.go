package mail

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// SMTPConfig is the outbound mail server configuration.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// Enabled reports whether the SMTP configuration is usable.
func (c SMTPConfig) Enabled() bool {
	return c.Host != "" && c.Port > 0
}

// Send delivers a plain-text message to a single recipient.
func (c SMTPConfig) Send(to, subject, body string) error {
	if !c.Enabled() {
		return fmt.Errorf("smtp is not configured")
	}
	from := c.From
	if from == "" {
		from = c.Username
	}
	address := net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	client, err := dial(address, c.Port, c.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}
	if c.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", c.Username, c.Password, c.Host)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n",
		from, to, encodeHeader(subject), body)
	if _, err := writer.Write([]byte(message)); err != nil {
		writer.Close()
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("smtp close: %w", err)
	}
	return client.Quit()
}

// dial opens a connection, using implicit TLS for the classic SMTPS port 465.
func dial(address string, port int, serverName string) (*smtp.Client, error) {
	connection, err := net.DialTimeout("tcp", address, 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("smtp dial: %w", err)
	}
	if port == 465 {
		connection = tls.Client(connection, &tls.Config{ServerName: serverName, MinVersion: tls.VersionTLS12})
	}
	client, err := smtp.NewClient(connection, serverName)
	if err != nil {
		connection.Close()
		return nil, fmt.Errorf("smtp handshake: %w", err)
	}
	return client, nil
}

// encodeHeader encodes a subject line as RFC 2047 UTF-8 so non-ASCII text survives.
func encodeHeader(value string) string {
	return "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(value))) + "?="
}
