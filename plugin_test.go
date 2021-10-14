package traefik_plugin_geoblock

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
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
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/foobar", nil)

		rr := httptest.NewRecorder()
		plugin.ServeHTTP(rr, req)

		require.Equal(t, http.StatusTeapot, rr.Code)
	})

	t.Run("NoNextHandler", func(t *testing.T) {
		plugin, err := New(context.TODO(), nil, &Config{Enabled: true}, pluginName)
		require.Error(t, err)
		require.Nil(t, plugin)
	})

	t.Run("NoConfig", func(t *testing.T) {
		plugin, err := New(context.TODO(), &noopHandler{}, nil, pluginName)
		require.Error(t, err)
		require.Nil(t, plugin)
	})

	t.Run("NoDatabaseFilePath", func(t *testing.T) {
		plugin, err := New(context.TODO(), &noopHandler{}, &Config{Enabled: true}, pluginName)
		require.Error(t, err)
		require.Nil(t, plugin)
	})
}

func TestPlugin_ServeHTTP(t *testing.T) {
	t.Run("Allowed", func(t *testing.T) {
		cfg := &Config{Enabled: true, DatabaseFilePath: dbFilePath, AllowedCountries: []string{"US"}}
		plugin, err := New(context.TODO(), &noopHandler{}, cfg, pluginName)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/foobar", nil)
		req.Header.Set("X-Real-IP", "1.1.1.1")

		rr := httptest.NewRecorder()
		plugin.ServeHTTP(rr, req)

		require.Equal(t, http.StatusTeapot, rr.Code)
	})

	t.Run("AllowedPrivate", func(t *testing.T) {
		cfg := &Config{Enabled: true, DatabaseFilePath: dbFilePath, AllowedCountries: []string{}, AllowPrivate: true}
		plugin, err := New(context.TODO(), &noopHandler{}, cfg, pluginName)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/foobar", nil)
		req.Header.Set("X-Real-IP", "192.168.178.66")

		rr := httptest.NewRecorder()
		plugin.ServeHTTP(rr, req)

		require.Equal(t, http.StatusTeapot, rr.Code)
	})

	t.Run("Disallowed", func(t *testing.T) {
		cfg := &Config{Enabled: true, DatabaseFilePath: dbFilePath, AllowedCountries: []string{"DE"}}
		plugin, err := New(context.TODO(), &noopHandler{}, cfg, pluginName)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/foobar", nil)
		req.Header.Set("X-Real-IP", "1.1.1.1")

		rr := httptest.NewRecorder()
		plugin.ServeHTTP(rr, req)

		require.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("DisallowedPrivate", func(t *testing.T) {
		cfg := &Config{Enabled: true, DatabaseFilePath: dbFilePath, AllowedCountries: []string{}, AllowPrivate: false}
		plugin, err := New(context.TODO(), &noopHandler{}, cfg, pluginName)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/foobar", nil)
		req.Header.Set("X-Real-IP", "192.168.178.66")

		rr := httptest.NewRecorder()
		plugin.ServeHTTP(rr, req)

		require.Equal(t, http.StatusForbidden, rr.Code)
	})
}
