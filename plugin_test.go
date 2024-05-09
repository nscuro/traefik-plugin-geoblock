package traefik_plugin_geoblock

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	pluginName = "geoblock"
	dbFilePath = "./IP2LOCATION-LITE-DB1.IPV6.BIN"
)

type noopHandler struct{}

func (n noopHandler) ServeHTTP(rw http.ResponseWriter, _ *http.Request) {
	rw.WriteHeader(http.StatusTeapot)
}

func TestNew(t *testing.T) {
	t.Run("Disabled", func(t *testing.T) {
		plugin, err := New(context.TODO(), &noopHandler{}, &Config{Enabled: false}, pluginName)
		if err != nil {
			t.Errorf("expected no error, but got: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/foobar", nil)

		rr := httptest.NewRecorder()
		plugin.ServeHTTP(rr, req)

		if rr.Code != http.StatusTeapot {
			t.Errorf("expected status code %d, but got: %d", http.StatusTeapot, rr.Code)
		}
	})

	t.Run("NoNextHandler", func(t *testing.T) {
		plugin, err := New(context.TODO(), nil, &Config{Enabled: true}, pluginName)
		if err == nil {
			t.Errorf("expected error, but got none")
		}
		if plugin != nil {
			t.Error("expected plugin to be nil, but is not")
		}
	})

	t.Run("Nogeoblock.Config", func(t *testing.T) {
		plugin, err := New(context.TODO(), &noopHandler{}, nil, pluginName)
		if err == nil {
			t.Errorf("expected error, but got none")
		}
		if plugin != nil {
			t.Error("expected plugin to be nil, but is not")
		}
	})

	t.Run("InvalidDisallowedStatusCode", func(t *testing.T) {
		plugin, err := New(context.TODO(), &noopHandler{}, &Config{Enabled: true, DisallowedStatusCode: -1}, pluginName)
		if err == nil {
			t.Errorf("expected error, but got none")
		}
		if plugin != nil {
			t.Error("expected plugin to be nil, but is not")
		}
	})

	t.Run("NoDatabaseFilePath", func(t *testing.T) {
		plugin, err := New(context.TODO(), &noopHandler{}, &Config{Enabled: true, DisallowedStatusCode: http.StatusForbidden}, pluginName)
		if err == nil {
			t.Errorf("expected error, but got none")
		}
		if plugin != nil {
			t.Error("expected plugin to be nil, but is not")
		}
	})
}

func TestPlugin_ServeHTTP(t *testing.T) {
	t.Run("Allowed", func(t *testing.T) {
		cfg := &Config{
			Enabled:              true,
			DatabaseFilePath:     dbFilePath,
			AllowedCountries:     []string{"US"},
			DisallowedStatusCode: http.StatusOK,
		}

		plugin, err := New(context.TODO(), &noopHandler{}, cfg, pluginName)
		if err != nil {
			t.Errorf("expected no error, but got: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/foobar", nil)
		req.Header.Set("X-Real-IP", "1.1.1.1")

		rr := httptest.NewRecorder()
		plugin.ServeHTTP(rr, req)

		if rr.Code != http.StatusTeapot {
			t.Errorf("expected status code %d, but got: %d", http.StatusTeapot, rr.Code)
		}
	})

	t.Run("AllowedPrivate", func(t *testing.T) {
		cfg := &Config{
			Enabled:              true,
			DatabaseFilePath:     dbFilePath,
			AllowedCountries:     []string{},
			DisallowedStatusCode: http.StatusOK,
			DefaultAllow:         true,
		}

		plugin, err := New(context.TODO(), &noopHandler{}, cfg, pluginName)
		if err != nil {
			t.Errorf("expected no error, but got: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/foobar", nil)
		req.Header.Set("X-Real-IP", "192.168.178.66")

		rr := httptest.NewRecorder()
		plugin.ServeHTTP(rr, req)

		if rr.Code != http.StatusTeapot {
			t.Errorf("expected status code %d, but got: %d", http.StatusTeapot, rr.Code)
		}
	})

	t.Run("Disallowed", func(t *testing.T) {
		cfg := &Config{
			Enabled:              true,
			DatabaseFilePath:     dbFilePath,
			AllowedCountries:     []string{"DE"},
			DisallowedStatusCode: http.StatusForbidden,
		}

		plugin, err := New(context.TODO(), &noopHandler{}, cfg, pluginName)
		if err != nil {
			t.Errorf("expected no error, but got: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/foobar", nil)
		req.Header.Set("X-Real-IP", "1.1.1.1")

		rr := httptest.NewRecorder()
		plugin.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("expected status code %d, but got: %d", http.StatusForbidden, rr.Code)
		}
	})

	t.Run("DisallowedPrivate", func(t *testing.T) {
		cfg := &Config{
			Enabled:              true,
			DatabaseFilePath:     dbFilePath,
			AllowedCountries:     []string{},
			AllowPrivate:         false,
			DisallowedStatusCode: http.StatusForbidden,
			DefaultAllow:         false,
		}

		plugin, err := New(context.TODO(), &noopHandler{}, cfg, pluginName)
		if err != nil {
			t.Errorf("expected no error, but got: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/foobar", nil)
		req.Header.Set("X-Real-IP", "192.168.178.66")

		rr := httptest.NewRecorder()
		plugin.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("expected status code %d, but got: %d", http.StatusForbidden, rr.Code)
		}
	})

	t.Run("Blocklist", func(t *testing.T) {
		cfg := &Config{
			Enabled:              true,
			DatabaseFilePath:     dbFilePath,
			BlockedCountries:     []string{"US"},
			AllowPrivate:         false,
			DefaultAllow:         true,
			DisallowedStatusCode: http.StatusForbidden,
		}

		testRequest(t, "US IP blocked", cfg, "8.8.8.8", http.StatusForbidden)
		testRequest(t, "DE IP allowed", cfg, "185.5.82.105", 0)

		cfg.BlockedCountries = nil
		cfg.BlockedIPBlocks = []string{"8.8.8.0/24"}

		testRequest(t, "Google DNS-A blocked", cfg, "8.8.8.8", http.StatusForbidden)
		testRequest(t, "Google DNS-B allowed", cfg, "8.8.4.4", 0)

		cfg.AllowedIPBlocks = []string{"8.8.8.7/32"}

		testRequest(t, "Higher specificity IP CIDR allow trumps lower specificity IP CIDR block", cfg, "8.8.8.7", 0)
		testRequest(t, "Higher specificity IP CIDR allow should not override encompassing CIDR block", cfg, "8.8.8.9", http.StatusForbidden)

		cfg.DefaultAllow = false

		testRequest(t, "Default allow false", cfg, "8.8.4.4", http.StatusForbidden)
	})
}

func testRequest(t *testing.T, testName string, cfg *Config, ip string, expectedStatus int) {
	t.Run(testName, func(t *testing.T) {
		plugin, err := New(context.TODO(), &noopHandler{}, cfg, pluginName)
		if err != nil {
			t.Errorf("expected no error, but got: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/foobar", nil)
		req.Header.Set("X-Real-IP", ip)

		rr := httptest.NewRecorder()
		plugin.ServeHTTP(rr, req)

		if expectedStatus > 0 && rr.Code != expectedStatus {
			t.Errorf("expected status code %d, but got: %d", expectedStatus, rr.Code)
		}
	})
}

func TestPlugin_Lookup(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		cfg := &Config{
			Enabled:              true,
			DatabaseFilePath:     dbFilePath,
			AllowedCountries:     []string{},
			AllowPrivate:         false,
			DisallowedStatusCode: http.StatusForbidden,
		}

		plugin, err := New(context.TODO(), &noopHandler{}, cfg, pluginName)
		if err != nil {
			t.Errorf("expected no error, but got: %v", err)
		}

		country, err := plugin.(*Plugin).Lookup("8.8.8.8")
		if err != nil {
			t.Errorf("expected no error, but got: %v", err)
		}
		if country != "US" {
			t.Errorf("expected country to be %s, but got: %s", "US", country)
		}
	})

	t.Run("Invalid", func(t *testing.T) {
		cfg := &Config{
			Enabled:              true,
			DatabaseFilePath:     dbFilePath,
			AllowedCountries:     []string{},
			AllowPrivate:         false,
			DisallowedStatusCode: http.StatusForbidden,
		}

		plugin, err := New(context.TODO(), &noopHandler{}, cfg, pluginName)
		if err != nil {
			t.Errorf("expected no error, but got: %v", err)
		}

		country, err := plugin.(*Plugin).Lookup("foobar")
		if err == nil {
			t.Errorf("expected error, but got none")
		}
		if err.Error() != "Invalid IP address." {
			t.Errorf("unexpected error: %v", err)
		}
		if country != "" {
			t.Errorf("expected country to be empty, but was: %s", country)
		}
	})
}
