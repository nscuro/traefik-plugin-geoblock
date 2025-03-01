package traefik_plugin_geoblock

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

	t.Run("UnableToFindDatabase", func(t *testing.T) {
		plugin, err := New(context.TODO(), &noopHandler{}, &Config{Enabled: true, DisallowedStatusCode: http.StatusForbidden, DatabaseFilePath: "bad-database"}, pluginName)
		if err == nil {
			t.Errorf("expected error, but got none")
		}
		if plugin != nil {
			t.Error("expected plugin to be nil, but is not")
		}
	})

	t.Run("InvalidDatabaseVersion", func(t *testing.T) {
		plugin, err := New(context.TODO(), &noopHandler{}, &Config{
			Enabled:          true,
			DatabaseFilePath: "./testdata/invalid.bin",
		}, pluginName)
		if err == nil {
			t.Errorf("expected error about invalid database version, but got none")
		}
		if plugin != nil {
			t.Error("expected plugin to be nil, but is not")
		}
	})
}

func TestNew_AutoUpdate(t *testing.T) {
	// Create a temporary directory for test databases
	tmpDir, err := os.MkdirTemp("", "geoblock-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// This is the default location for the internal temp copy
	tmpFile := filepath.Join(os.TempDir(), "IP2LOCATION-LITE-DB1.IPV6.BIN")
	_ = os.Remove(tmpFile)

	// Copy the test database to the temp directory with a versioned name
	versionedDbPath := filepath.Join(tmpDir, "20240301_IP2LOCATION-LITE-DB1.IPV6.BIN")
	if err := copyFile(dbFilePath, versionedDbPath, true); err != nil {
		t.Fatalf("Failed to copy test database: %v", err)
	}

	t.Run("AutoUpdateEnabled", func(t *testing.T) {
		cfg := &Config{
			Enabled:                true,
			DatabaseAutoUpdate:     true,
			DatabaseAutoUpdateDir:  tmpDir,
			DatabaseAutoUpdateCode: "DB1",
			DisallowedStatusCode:   http.StatusForbidden,
		}

		plugin, err := New(context.TODO(), &noopHandler{}, cfg, pluginName)
		if err != nil {
			t.Errorf("expected no error, but got: %v", err)
		}
		if plugin == nil {
			t.Error("expected plugin to not be nil")
		}

		// We have no reference to what database file was actually used....
		p := plugin.(*Plugin)
		if p.databaseFile != tmpFile {
			t.Error("expected database to be initialized")
		}
	})

	t.Run("AutoUpdateEnabledNoDir", func(t *testing.T) {
		cfg := &Config{
			Enabled:              true,
			DatabaseAutoUpdate:   true,
			DisallowedStatusCode: http.StatusForbidden,
			// Deliberately omit DatabaseAutoUpdateDir
		}

		plugin, err := New(context.TODO(), &noopHandler{}, cfg, pluginName)
		if err == nil {
			t.Error("expected error about missing DatabaseAutoUpdateDir, but got none")
		}
		if plugin != nil {
			t.Error("expected plugin to be nil")
		}
	})

	t.Run("AutoUpdateEnabledEmptyDir", func(t *testing.T) {
		emptyDir, err := os.MkdirTemp("", "geoblock-empty-*")
		if err != nil {
			t.Fatalf("Failed to create empty temp dir: %v", err)
		}
		defer os.RemoveAll(emptyDir)

		cfg := &Config{
			Enabled:               true,
			DatabaseAutoUpdate:    true,
			DatabaseAutoUpdateDir: emptyDir,
			DisallowedStatusCode:  http.StatusForbidden,
			DatabaseFilePath:      dbFilePath, // Fall back to default database
		}

		plugin, err := New(context.TODO(), &noopHandler{}, cfg, pluginName)
		if err != nil {
			t.Errorf("expected no error when falling back to default database, but got: %v", err)
		}
		if plugin == nil {
			t.Error("expected plugin to not be nil when falling back to default database")
		}
	})
}

func TestPlugin_ServeHTTP(t *testing.T) {
	t.Run("Allowed", func(t *testing.T) {
		cfg := &Config{
			Enabled:              true,
			DatabaseFilePath:     dbFilePath,
			AllowedCountries:     []string{"AU"},
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
			AllowPrivate:         true,
			DisallowedStatusCode: http.StatusOK,
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

	t.Run("AllowedPrivate172Range", func(t *testing.T) {
		cfg := &Config{
			Enabled:              true,
			DatabaseFilePath:     dbFilePath,
			AllowedCountries:     []string{},
			AllowPrivate:         true,
			DisallowedStatusCode: http.StatusOK,
		}

		plugin, err := New(context.TODO(), &noopHandler{}, cfg, pluginName)
		if err != nil {
			t.Errorf("expected no error, but got: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/foobar", nil)
		req.Header.Set("X-Real-IP", "172.19.0.1")

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
			BanHtmlFilePath:      "geoblockban.html",
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

		// Check that response contains the IP address
		body := rr.Body.String()
		if !strings.Contains(body, "1.1.1.1") {
			t.Errorf("expected response to contain IP address '1.1.1.1', but got: %s", body)
		}
	})

	t.Run("DisallowedPrivate", func(t *testing.T) {
		cfg := &Config{
			Enabled:              true,
			DatabaseFilePath:     dbFilePath,
			AllowedCountries:     []string{},
			AllowPrivate:         false,
			DisallowedStatusCode: http.StatusForbidden,
			BanHtmlFilePath:      "geoblockban.html",
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

		// Check that response contains the IP address
		body := rr.Body.String()
		if !strings.Contains(body, "192.168.178.66") {
			t.Errorf("expected response to contain IP address '192.168.178.66', but got: %s", body)
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

	t.Run("IPWhitelist", func(t *testing.T) {
		cfg := &Config{
			Enabled:              true,
			DatabaseFilePath:     dbFilePath,
			AllowedIPBlocks:      []string{"203.0.113.0/24", "198.51.100.1/32"}, // Using TEST-NET-3 and TEST-NET-2 ranges
			DefaultAllow:         false,
			DisallowedStatusCode: http.StatusForbidden,
		}

		testRequest(t, "Whitelisted subnet allowed", cfg, "203.0.113.100", http.StatusTeapot)
		testRequest(t, "Whitelisted specific IP allowed", cfg, "198.51.100.1", http.StatusTeapot)
		testRequest(t, "Non-whitelisted IP blocked", cfg, "203.0.114.1", http.StatusForbidden)

		// Test interaction with country rules
		cfg.AllowedCountries = []string{"US"}
		testRequest(t, "Whitelisted IP allowed despite country rules", cfg, "203.0.113.100", http.StatusTeapot)
		testRequest(t, "US IP allowed when in allowed countries", cfg, "8.8.8.8", http.StatusTeapot)
		testRequest(t, "Non-US IP blocked when not in whitelist", cfg, "1.1.1.1", http.StatusForbidden)
	})

	t.Run("BypassHeaders", func(t *testing.T) {
		cfg := &Config{
			Enabled:              true,
			DatabaseFilePath:     dbFilePath,
			BlockedCountries:     []string{"US"},
			DisallowedStatusCode: http.StatusForbidden,
			BypassHeaders: map[string]string{
				"X-Bypass-Key": "secret123",
				"Auth-Token":   "bypass-token",
			},
		}

		tests := []struct {
			name         string
			headers      map[string]string
			expectedCode int
			description  string
		}{
			{
				name: "bypass with correct X-Bypass-Key",
				headers: map[string]string{
					"X-Real-IP":    "8.8.8.8", // US IP that would normally be blocked
					"X-Bypass-Key": "secret123",
				},
				expectedCode: http.StatusTeapot,
				description:  "should allow access with correct bypass header",
			},
			{
				name: "bypass with correct Auth-Token",
				headers: map[string]string{
					"X-Real-IP":  "8.8.8.8",
					"Auth-Token": "bypass-token",
				},
				expectedCode: http.StatusTeapot,
				description:  "should allow access with correct auth token",
			},
			{
				name: "no bypass with incorrect header value",
				headers: map[string]string{
					"X-Real-IP":    "8.8.8.8",
					"X-Bypass-Key": "wrong-secret",
				},
				expectedCode: http.StatusForbidden,
				description:  "should block access with incorrect bypass header",
			},
			{
				name: "no bypass without headers",
				headers: map[string]string{
					"X-Real-IP": "8.8.8.8",
				},
				expectedCode: http.StatusForbidden,
				description:  "should block access without bypass headers",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				plugin, err := New(context.TODO(), &noopHandler{}, cfg, pluginName)
				if err != nil {
					t.Fatalf("expected no error, but got: %v", err)
				}

				req := httptest.NewRequest(http.MethodGet, "/foobar", nil)
				for key, value := range tt.headers {
					req.Header.Set(key, value)
				}

				rr := httptest.NewRecorder()
				plugin.ServeHTTP(rr, req)

				if rr.Code != tt.expectedCode {
					t.Errorf("%s: expected status code %d, but got: %d",
						tt.description, tt.expectedCode, rr.Code)
				}
			})
		}
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

func TestPlugin_ServeHTTP_MalformedIP(t *testing.T) {
	tests := []struct {
		name       string
		banIfError bool
		ip         string
		wantStatus int
	}{
		{
			name:       "malformed IP with banIfError true",
			banIfError: true,
			ip:         "not.an.ip.address",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "malformed IP with banIfError false",
			banIfError: false,
			ip:         "not.an.ip.address",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test response recorder
			rr := httptest.NewRecorder()

			// Create a test request with the malformed IP
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("X-Forwarded-For", tt.ip)

			// Create a mock next handler that always returns 200 OK
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			// Create plugin config
			cfg := &Config{
				Enabled:              true,
				DisallowedStatusCode: http.StatusForbidden,
				BanIfError:           tt.banIfError,
				DatabaseFilePath:     dbFilePath,
			}

			// Initialize plugin
			plugin, err := New(context.Background(), next, cfg, "test")
			if err != nil {
				t.Fatalf("Failed to create plugin: %v", err)
			}

			// Serve the request
			plugin.ServeHTTP(rr, req)

			// Check the status code
			if rr.Code != tt.wantStatus {
				t.Errorf("ServeHTTP() status = %v, want %v", rr.Code, tt.wantStatus)
			}
		})
	}
}
