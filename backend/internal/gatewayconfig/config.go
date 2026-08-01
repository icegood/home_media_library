package gatewayconfig

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/fs"
	"net/mail"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"media-library/backend/internal/domain"
	"media-library/backend/internal/store"
)

const letsEncryptCA = "https://acme-v02.api.letsencrypt.org/directory"

var dnsName = regexp.MustCompile(`(?i)^([a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

type Settings struct {
	HTTPEnabled  bool   `json:"httpEnabled"`
	HTTPSEnabled bool   `json:"httpsEnabled"`
	PublicDNS    string `json:"publicDns"`
	ACMEEmail    string `json:"acmeEmail"`
}

func Load(ctx context.Context, repository store.Store) Settings {
	settings, err := repository.ServerSettings(ctx)
	if err != nil {
		settings = domain.DefaultServerSettings()
	}
	return FromServerSettings(settings)
}

func FromServerSettings(settings domain.ServerSettings) Settings {
	return Settings{
		HTTPEnabled:  settings.HTTPEnabled,
		HTTPSEnabled: settings.HTTPSEnabled,
		PublicDNS:    settings.PublicDNS,
		ACMEEmail:    settings.ACMEEmail,
	}
}

func Validate(settings Settings) error {
	if !settings.HTTPEnabled && !settings.HTTPSEnabled {
		return fmt.Errorf("at least one of HTTP or HTTPS must be enabled")
	}
	if settings.HTTPSEnabled {
		settings.PublicDNS = strings.TrimSpace(settings.PublicDNS)
		if !dnsName.MatchString(settings.PublicDNS) {
			return fmt.Errorf("a valid public DNS name is required for HTTPS")
		}
		address, err := mail.ParseAddress(strings.TrimSpace(settings.ACMEEmail))
		if err != nil || address.Address != strings.TrimSpace(settings.ACMEEmail) {
			return fmt.Errorf("a valid Let's Encrypt email is required for HTTPS")
		}
	}
	return nil
}

func Write(path string, settings Settings) error {
	if err := Validate(settings); err != nil {
		return err
	}
	var config strings.Builder
	config.WriteString("{\n\tauto_https disable_redirects\n")
	if settings.HTTPSEnabled {
		fmt.Fprintf(&config, "\temail %s\n\tacme_ca %s\n", settings.ACMEEmail, letsEncryptCA)
	}
	config.WriteString("}\n\n")
	if settings.HTTPEnabled {
		config.WriteString("http://:80 {\n\treverse_proxy web:8080 {\n\t\tflush_interval -1\n\t}\n}\n\n")
	}
	if settings.HTTPSEnabled {
		fmt.Fprintf(&config, "%s {\n\ttls %s\n\theader {\n\t\tStrict-Transport-Security \"max-age=31536000; includeSubDomains\"\n\t\tX-Frame-Options \"DENY\"\n\t\t-Server\n\t}\n\treverse_proxy web:8080 {\n\t\tflush_interval -1\n\t}\n}\n", settings.PublicDNS, settings.ACMEEmail)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".Caddyfile-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if _, err := temp.WriteString(config.String()); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func CertificateExpiration(dataDir, publicDNS string) (time.Time, bool) {
	publicDNS = strings.TrimSpace(publicDNS)
	if dataDir == "" || publicDNS == "" {
		return time.Time{}, false
	}
	var newest time.Time
	found := false
	needle := strings.ToLower(publicDNS)
	_ = filepath.WalkDir(dataDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		name := strings.ToLower(entry.Name())
		if !strings.HasSuffix(name, ".crt") && !strings.HasSuffix(name, ".pem") {
			return nil
		}
		if !strings.Contains(strings.ToLower(path), needle) {
			return nil
		}
		if expires, ok := certificateFileExpiration(path); ok && (!found || expires.After(newest)) {
			newest = expires
			found = true
		}
		return nil
	})
	return newest, found
}

func certificateFileExpiration(path string) (time.Time, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}, false
	}
	for {
		block, rest := pem.Decode(data)
		if block == nil {
			break
		}
		data = rest
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err == nil {
			return cert.NotAfter, true
		}
	}
	return time.Time{}, false
}
