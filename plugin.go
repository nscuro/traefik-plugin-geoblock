package geoblock

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"github.com/ip2location/ip2location-go/v9"
	"log"
	"net/http"
	"strings"
)

type Config struct {
	DatabaseFilePath string
	AllowedCountries []string `yaml:"allowed_countries"`
}

func CreateConfig() *Config {
	return &Config{}
}

type Plugin struct {
	next             http.Handler
	name             string
	db               *ip2location.DB
	allowedCountries []string
}

func New(_ context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	db, err := ip2location.OpenDB(config.DatabaseFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	return &Plugin{next: next, name: name, db: db, allowedCountries: config.AllowedCountries}, nil
}

func (p Plugin) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	ips := p.GetRemoteIPs(req)

	for _, ip := range ips {
		allowed, country, err := p.CheckAllowed(ip)
		if err != nil {
			log.Printf("%s: %v", p.name, err)
			rw.WriteHeader(http.StatusForbidden)
			return
		}
		if !allowed {
			log.Printf("%s: access denied for %s (%s)", p.name, ip, country)
			rw.WriteHeader(http.StatusForbidden)
			return
		}
	}

	p.next.ServeHTTP(rw, req)
}

// GetRemoteIPs collects the remote IPs from the X-Forwarded-For and X-Real-IP headers.
func (p Plugin) GetRemoteIPs(req *http.Request) (ips []string) {
	ipMap := make(map[string]struct{})

	if xff := req.Header.Get("x-forwarded-for"); xff != "" {
		for _, ip := range strings.Split(xff, ",") {
			ip = strings.TrimSpace(ip)
			if ip == "" {
				continue
			}
			ipMap[ip] = struct{}{}
		}
	}
	if xri := req.Header.Get("x-real-ip"); xri != "" {
		for _, ip := range strings.Split(xri, ",") {
			ip = strings.TrimSpace(ip)
			if ip == "" {
				continue
			}
			ipMap[ip] = struct{}{}
		}
	}

	for ip := range ipMap {
		ips = append(ips, ip)
	}

	return
}

// CheckAllowed checks whether a given IP address is allowed according to the configured allowed countries.
func (p Plugin) CheckAllowed(ip string) (bool, string, error) {
	country, err := p.Lookup(ip)
	if err != nil {
		return false, "", fmt.Errorf("lookup of %s failed: %w", ip, err)
	}

	var allowed bool
	for _, allowedCountry := range p.allowedCountries {
		if allowedCountry == country {
			allowed = true
			break
		}
	}

	return allowed, country, nil
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
