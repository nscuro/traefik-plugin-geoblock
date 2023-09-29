package traefik_plugin_geoblock

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"net"
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
	BlockedCountries     []string // Blocklist of countries to be blocked (ISO 3166-1 alpha-2)
	DefaultAllow         bool     // If source matches neither blocklist nor whitelist, should it be allowed through?
	AllowPrivate         bool     // Allow requests from private / internal networks?
	DisallowedStatusCode int      // HTTP status code to return for disallowed requests
	AllowedIPBlocks      []string // List of whitelist CIDR
	BlockedIPBlocks      []string // List of blocklisted CIDRs
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
	blockedCountries     []string
	defaultAllow         bool
	allowPrivate         bool
	disallowedStatusCode int
	allowedIPBlocks      []*net.IPNet
	blockedIPBlocks      []*net.IPNet
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

	allowedIPBlocks, err := initIPBlocks(cfg.AllowedIPBlocks)
	if err != nil {
		return nil, fmt.Errorf("%s: failed loading allowed CIDR blocks: %w", name, err)
	}

	blockedIPBlocks, err := initIPBlocks(cfg.BlockedIPBlocks)
	if err != nil {
		return nil, fmt.Errorf("%s: failed loading allowed CIDR blocks: %w", name, err)
	}

	return &Plugin{
		next:                 next,
		name:                 name,
		db:                   db,
		enabled:              cfg.Enabled,
		allowedCountries:     cfg.AllowedCountries,
		blockedCountries:     cfg.BlockedCountries,
		defaultAllow:         cfg.DefaultAllow,
		allowPrivate:         cfg.AllowPrivate,
		disallowedStatusCode: cfg.DisallowedStatusCode,
		allowedIPBlocks:      allowedIPBlocks,
		blockedIPBlocks:      blockedIPBlocks,
	}, nil
}

// ServeHTTP implements the http.Handler interface.
func (p Plugin) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if !p.enabled {
		p.next.ServeHTTP(rw, req)
		return
	}

	for _, ip := range p.GetRemoteIPs(req) {
		allowed, country, err := p.CheckAllowed(ip)
		if err != nil {
			log.Printf("%s: [%s %s %s] - %v", p.name, req.Host, req.Method, req.URL.Path, err)
			rw.WriteHeader(p.disallowedStatusCode)
			return
		}
		if !allowed {
			log.Printf("%s: [%s %s %s] blocked request from %s", p.name, req.Host, req.Method, req.URL.Path, country)
			rw.WriteHeader(p.disallowedStatusCode)
			return
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

// CheckAllowed checks whether a given IP address is allowed according to the configured allowed countries.
func (p Plugin) CheckAllowed(ip string) (allow bool, country string, err error) {
	var allowedCountry, allowedIP, blockedCountry, blockedIP bool
	var allowedNetworkLength, blockedNetworkLength int

	country, err = p.Lookup(ip)
	if err != nil {
		return false, ip, fmt.Errorf("lookup of %s failed: %w", ip, err)
	}

	if country == "-" {
		return p.allowPrivate, country, nil
	}

	if country != "-" {
		for _, item := range p.blockedCountries {
			if item == country {
				blockedCountry = true

				break
			}
		}

		for _, item := range p.allowedCountries {
			if item == country {
				allowedCountry = true
			}
		}
	}

	blocked, blockedNetworkLength, err := p.isBlockedIPBlocks(ip)
	if err != nil {
		return false, ip, fmt.Errorf("failed to check if IP %q is blocked by IP block: %w", ip, err)
	}

	if blocked {
		blockedIP = true
	}

	for _, allowedCountry := range p.allowedCountries {
		if allowedCountry == country {
			return true, ip, nil
		}
	}

	allowed, allowedNetBits, err := p.isAllowedIPBlocks(ip)
	if err != nil {
		return false, ip, fmt.Errorf("failed to check if IP %q is allowed by IP block: %w", ip, err)
	}

	if allowed {
		allowedIP = true
		allowedNetworkLength = allowedNetBits
	}

	// Handle final values
	//
	// NB: discrete IPs have higher priority than countries:  more specific to less specific.

	// NB: whichever matched prefix is longer has higher priority: more specific to less specific.
	if allowedNetworkLength < blockedNetworkLength {
		if blockedIP {
			return false, country, nil
		}

		if allowedIP {
			return true, country, nil
		}
	} else {
		if allowedIP {
			return true, country, nil
		}

		if blockedIP {
			return false, country, nil
		}
	}

	if allowedCountry {
		return true, country, nil
	}

	if blockedCountry {
		return false, country, nil
	}

	return p.defaultAllow, country, nil
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

// Create IP Networks using CIDR block array
func initIPBlocks(ipBlocks []string) ([]*net.IPNet, error) {

	var ipBlocksNet []*net.IPNet

	for _, cidr := range ipBlocks {
		_, block, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("parse error on %q: %v", cidr, err)
		}
		ipBlocksNet = append(ipBlocksNet, block)
	}

	return ipBlocksNet, nil
}

// isAllowedIPBlocks checks if an IP is allowed base on the allowed CIDR blocks
func (p Plugin) isAllowedIPBlocks(ip string) (bool, int, error) {
	return p.isInIPBlocks(ip, p.allowedIPBlocks)
}

// isBlockedIPBlocks checks if an IP is allowed base on the blocked CIDR blocks
func (p Plugin) isBlockedIPBlocks(ip string) (bool, int, error) {
	return p.isInIPBlocks(ip, p.blockedIPBlocks)
}

// isInIPBlocks indicates whether the given IP exists in any of the IP subnets contained within ipBlocks.
func (p Plugin) isInIPBlocks(ip string, ipBlocks []*net.IPNet) (bool, int, error) {
	ipAddress := net.ParseIP(ip)

	if ipAddress == nil {
		return false, 0, fmt.Errorf("unable parse IP address from address [%s]", ip)
	}

	for _, block := range ipBlocks {
		if block.Contains(ipAddress) {
			ones, _ := block.Mask.Size()

			return true, ones, nil
		}
	}

	return false, 0, nil
}
