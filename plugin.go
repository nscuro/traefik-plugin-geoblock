package traefik_plugin_geoblock

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/ip2location/ip2location-go/v9"
)

//go:generate go run ./tools/dbdownload/main.go -o ./IP2LOCATION-LITE-DB1.IPV6.BIN

// Config defines the plugin configuration.
type Config struct {
	Enabled              bool     // Enable this plugin?
	DatabaseFilePath     string   // Path to ip2location database file
	AllowedCountries     []string // Whitelist of countries to allow (ISO 3166-1 alpha-2)
	AllowPrivate         bool     // Allow requests from private / internal networks?
	DisallowedStatusCode int      // HTTP status code to return for disallowed requests
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{DisallowedStatusCode: http.StatusForbidden}
}

type Plugin struct {
	next                 http.Handler
	name                 string
	db                   *ip2location.DB
	enabled              bool
	allowedCountries     []string
	allowPrivate         bool
	disallowedStatusCode int
}

// New creates a new plugin instance.
func New(_ context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	if next == nil {
		return nil, fmt.Errorf("%s: no next handler provided", name)
	}
	if cfg == nil {
		return nil, fmt.Errorf("%s: no config provided", name)
	}

	if !cfg.Enabled {
		log.Printf("%s: disabled", name)

		return &Plugin{
			next: next,
			name: name,
			db:   nil,
		}, nil
	}

	if http.StatusText(cfg.DisallowedStatusCode) == "" {
		return nil, fmt.Errorf("%s: %d is not a valid http status code", name, cfg.DisallowedStatusCode)
	}

	if cfg.DatabaseFilePath == "" {
		return nil, fmt.Errorf("%s: no database file path configured", name)
	}

	db, err := ip2location.OpenDB(cfg.DatabaseFilePath)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to open database: %w", name, err)
	}

	return &Plugin{
		next:                 next,
		name:                 name,
		db:                   db,
		enabled:              cfg.Enabled,
		allowedCountries:     cfg.AllowedCountries,
		allowPrivate:         cfg.AllowPrivate,
		disallowedStatusCode: cfg.DisallowedStatusCode,
	}, nil
}

// ServeHTTP implements the http.Handler interface.
func (p Plugin) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if !p.enabled {
		p.next.ServeHTTP(rw, req)
		return
	}

	for _, ip := range p.GetRemoteIPs(req) {
		err := p.CheckAllowed(ip)
		if err != nil {
			var notAllowedErr NotAllowedError
			if errors.As(err, &notAllowedErr) {
				log.Printf("%s: %v", p.name, err)
				rw.WriteHeader(p.disallowedStatusCode)
				return
			} else {
				log.Printf("%s: %s - %v", p.name, req.Host, err)
				rw.WriteHeader(p.disallowedStatusCode)
				return
			}
		}
	}

	p.next.ServeHTTP(rw, req)
}

// GetRemoteIPs collects the remote IPs from the X-Forwarded-For and X-Real-IP headers.
func (p Plugin) GetRemoteIPs(req *http.Request) []string {
	uniqIPs := make(map[string]struct{})

	if xff := req.Header.Get("x-forwarded-for"); xff != "" {
		for _, ip := range strings.Split(xff, ",") {
			ip = strings.TrimSpace(ip)
			if ip == "" {
				continue
			}
			uniqIPs[ip] = struct{}{}
		}
	}
	if xri := req.Header.Get("x-real-ip"); xri != "" {
		for _, ip := range strings.Split(xri, ",") {
			ip = strings.TrimSpace(ip)
			if ip == "" {
				continue
			}
			uniqIPs[ip] = struct{}{}
		}
	}

	var ips []string
	for ip := range uniqIPs {
		ips = append(ips, ip)
	}

	return ips
}

type NotAllowedError struct {
	Country string
	IP      string
	Reason  string
}

func (e NotAllowedError) Error() (err string) {
	if e.Country == "" {
		err = fmt.Sprintf("%s not allowed", e.IP)
	} else {
		err = fmt.Sprintf("%s (%s) not allowed", e.IP, e.Country)
	}
	if e.Reason != "" {
		err = fmt.Sprintf("%s: %s", err, e.Reason)
	}

	return err
}

// CheckAllowed checks whether a given IP address is allowed according to the configured allowed countries.
func (p Plugin) CheckAllowed(ip string) error {
	country, err := p.Lookup(ip)
	if err != nil {
		return fmt.Errorf("lookup of %s failed: %w", ip, err)
	}

	if country == "-" { // Private address
		if p.allowPrivate {
			return nil
		}

		return NotAllowedError{
			IP:     ip,
			Reason: "private address",
		}
	}

	var allowed bool
	for _, allowedCountry := range p.allowedCountries {
		if allowedCountry == country {
			allowed = true
			break
		}
	}
	if !allowed {
		return NotAllowedError{
			Country: country,
			IP:      ip,
		}
	}

	return nil
}

// Lookup queries the ip2location database for a given IP address.
func (p Plugin) Lookup(ip string) (string, error) {
	record, err := p.db.Get_country_short(ip)
	if err != nil {
		return "", err
	}

	country := record.Country_short
	if strings.HasPrefix(strings.ToLower(country), "invalid") {
		return "", errors.New(country)
	}

	return record.Country_short, nil
}
