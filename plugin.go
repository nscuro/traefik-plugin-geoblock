package traefik_plugin_geoblock

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"log/slog"

	"github.com/ip2location/ip2location-go/v9"
)

//go:generate go run ./tools/dbdownload/main.go -o ./IP2LOCATION-LITE-DB1.IPV6.BIN

// Add this constant near the top of the file, after imports
const PrivateIpCountryAlias = "PRIVATE"

// Config defines the plugin configuration.
type Config struct {
	// Core settings
	Enabled          bool   // Enable/disable the plugin
	DatabaseFilePath string // Path to ip2location database file
	DefaultAllow     bool   // Default behavior when IP matches no rules
	AllowPrivate     bool   // Allow requests from private/internal networks
	BanIfError       bool   // Ban requests if IP lookup fails

	// Country-based rules (ISO 3166-1 alpha-2 format)
	AllowedCountries []string // Whitelist of countries to allow
	BlockedCountries []string // Blocklist of countries to block

	// IP-based rules
	AllowedIPBlocks []string // Whitelist of CIDR blocks
	BlockedIPBlocks []string // Blocklist of CIDR blocks

	// Response settings
	DisallowedStatusCode int    // HTTP status code for blocked requests
	BanHtmlFilePath      string // Custom HTML template for blocked requests

	// Logging configuration
	LogLevel          string // Log level: "debug", "info", "warn", "error"
	LogFormat         string // Log format: "json" or "text"
	LogPath           string // Log destination: "stdout", "stderr", or file path
	LogBannedRequests bool   // Log blocked requests

	// BypassHeaders is a map of header names to values that, when matched,
	// will skip the geoblocking check entirely
	BypassHeaders map[string]string

	// Auto-update settings
	DatabaseAutoUpdate      bool   `json:"databaseAutoUpdate,omitempty"`
	DatabaseAutoUpdateDir   string `json:"databaseAutoUpdateDir,omitempty"`
	DatabaseAutoUpdateToken string `json:"databaseAutoUpdateToken,omitempty"`
	DatabaseAutoUpdateCode  string `json:"databaseAutoUpdateCode,omitempty"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{
		DisallowedStatusCode:   http.StatusForbidden,
		LogLevel:               "info",                  // Default to info logging
		LogFormat:              "text",                  // Default to text format
		LogPath:                "",                      // Default to traefik
		BanIfError:             true,                    // Default to banning on errors
		BypassHeaders:          make(map[string]string), // Initialize empty map
		DatabaseAutoUpdateCode: "DB1",                   // Default database code
		LogBannedRequests:      true,                    // Default to logging blocked requests
	}
}

// Update the Plugin struct to store the ban HTML content instead of template
type Plugin struct {
	next                 http.Handler
	name                 string
	databaseFile         string // Just for testing purposes
	db                   *ip2location.DB
	enabled              bool
	allowedCountries     map[string]struct{} // Instead of []string to improve lookup performance
	blockedCountries     map[string]struct{} // Instead of []string to improve lookup performance
	defaultAllow         bool
	allowPrivate         bool
	banIfError           bool
	disallowedStatusCode int
	allowedIPBlocks      []*net.IPNet
	blockedIPBlocks      []*net.IPNet
	banHtmlContent       string // Changed from banHtmlTemplate
	logger               *slog.Logger
	bypassHeaders        map[string]string
	logBannedRequests    bool
}

func createBootstrapLogger(name string) *slog.Logger {
	var logLevel slog.Level = slog.LevelDebug

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	// Create a custom writer that uses fmt.Printf
	fmtWriter := &traefikLogWriter{}
	var writer io.Writer = fmtWriter
	handler := slog.NewTextHandler(writer, opts)
	return slog.New(handler).With("plugin", name)
}

// Update createLogger to use simpleFileWriter
func createLogger(name, level, format, path string, bootstrapLogger *slog.Logger) *slog.Logger {
	var logLevel slog.Level
	level = strings.ToLower(level) // Convert level to lowercase
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
		if level != "" {
			bootstrapLogger.Warn("Unknown log level", "level", level)
		}
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	// Create a custom writer that uses fmt.Printf
	fmtWriter := &traefikLogWriter{}
	var writer io.Writer = fmtWriter
	var destination string = "traefik"

	// Only attempt file writing if explicitly specified
	if path != "" {
		bw, err := newBufferedFileWriter(path, 1024, 2*time.Second)
		if err != nil {
			bootstrapLogger.Error("Failed to create buffered file writer for path '%s': %v\n", path, err)
		} else {
			writer = bw
			destination = path
		}
	}

	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(writer, opts)
	} else {
		handler = slog.NewTextHandler(writer, opts)
		format = "text" // normalize format name
	}

	// This log here so that in the traefik logs we see where are the logs actually going to for the middleware
	if logLevel <= slog.LevelDebug {
		bootstrapLogger.Debug(fmt.Sprintf("Logging to %s with %s format at %s level", destination, format, logLevel))
	}
	return slog.New(handler).With("plugin", name)
}

// traefikLogWriter implements io.Writer and uses fmt.Printf for output
type traefikLogWriter struct{}

func (w *traefikLogWriter) Write(p []byte) (n int, err error) {
	// https://github.com/traefik/traefik/issues/8204
	// Since v2.5.5, fmt.Println()/fmt.Printf() are catched and transfered to the Traefik logs, it's not perfect but we will improve that.
	log.Println(string(p))
	return len(p), nil
}

// searchFile looks for a file in the filesystem, handling both direct paths and directory searches.
// If baseFile is a direct path to an existing file, that path is returned.
// If baseFile is a directory, it recursively searches for defaultFile within that directory.
//
// Parameters:
//   - baseFile: Either a direct file path or directory to search in
//   - defaultFile: Filename to search for if baseFile is a directory
//
// Returns:
//   - The path to the found file, or the original baseFile path if not found
//
// The function will log errors if the file cannot be found or if there are issues during the search,
// but will not fail - it always returns a path.
func searchFile(baseFile string, defaultFile string, logger *slog.Logger) string {

	// Return early if baseFile is empty
	if baseFile == "" {
		return defaultFile
	}

	// Check if the file exists at the specified path
	if fileExists(baseFile) {
		return baseFile
	}

	err := filepath.Walk(baseFile, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors and continue walking
		}
		if !info.IsDir() {
			if filepath.Base(path) == defaultFile {
				baseFile = path         // Update baseFile with the found path
				return filepath.SkipAll // Stop walking once found
			}
		}
		return nil
	})

	if err != nil {
		// Log error but continue with original path
		logger.Error("error searching for file", "error", err)
	}

	if !fileExists(baseFile) {
		logger.Error("could not find file", "file", defaultFile, "path", baseFile)
	}

	return baseFile // Return found path or original path if not found
}

// New creates a new plugin instance.
func New(ctx context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	bootstrapLogger := createBootstrapLogger(name)

	if next == nil {
		return nil, fmt.Errorf("%s: no next handler provided", name)
	}

	if cfg == nil {
		return nil, fmt.Errorf("%s: no config provided", name)
	}

	// Create logger first so we can use it for debugging
	logger := createLogger(name, cfg.LogLevel, cfg.LogFormat, cfg.LogPath, bootstrapLogger)
	logger.Debug("initializing plugin",
		"logLevel", cfg.LogLevel,
		"logFormat", cfg.LogFormat,
		"logPath", cfg.LogPath)

	if !cfg.Enabled {
		bootstrapLogger.Warn("plugin disabled")
		return &Plugin{
			next:    next,
			name:    name,
			db:      nil,
			enabled: false,
			logger:  logger,
		}, nil
	}

	if http.StatusText(cfg.DisallowedStatusCode) == "" {
		return nil, fmt.Errorf("%s: %d is not a valid http status code", name, cfg.DisallowedStatusCode)
	}

	// Search for database file in plugin directories if path is provided. Even if auto-update is enabled this
	// might be a fallback location.
	if cfg.DatabaseFilePath != "" {
		cfg.DatabaseFilePath = searchFile(cfg.DatabaseFilePath, "IP2LOCATION-LITE-DB1.IPV6.BIN", bootstrapLogger)
	}

	// Handle auto-update configuration. This must be carefully crafted because multiple middleware instances
	// might be running at the same time and trying to compete here.
	if cfg.DatabaseAutoUpdate {
		if cfg.DatabaseAutoUpdateDir == "" {
			return nil, fmt.Errorf("DatabaseAutoUpdateDir must be specified when auto-update is enabled")
		}

		tmpFile := filepath.Join(os.TempDir(), "IP2LOCATION-LITE-DB1.IPV6.BIN")

		newDatabase := ""

		if fileExists(tmpFile) {
			// The traefik instance was already initialized previously or the middleware might have been refreshed.
			logger.Debug("Using already existing database from temp location")
			newDatabase = tmpFile
		} else {
			// Try to find and use latest database.
			if latest, err := findLatestDatabase(cfg.DatabaseAutoUpdateDir, cfg.DatabaseAutoUpdateCode); err == nil && latest != "" {
				// Copy database to temporary location. We can safely assume that the underlying storage defined in DatabaseAutoUpdateDir is NFS so it is
				// probably orders of magnitude slower in terms of IO than wherever the container itself is, which is probably
				// fast ephemeral storage in the node itself (i.e. in a Kubernetes cluster). If we have multiple middlewares
				// in the same traefik instance they will all compete to do this at startup, but i really DO NOT WANT to add
				// any sort of synchronization or lock during startup.
				if err := copyFile(latest, tmpFile, false); err != nil {
					logger.Warn(fmt.Sprintf("failed to copy database to temp location: %v", err))
					if fileExists(tmpFile) {
						// Other middlware initialization might have already copied the file, so we can use it.
						newDatabase = tmpFile
					} else {
						newDatabase = latest // Fallback to original location
					}
				} else {
					newDatabase = tmpFile
					logger.Debug(fmt.Sprintf("copied database to temp location: %s", tmpFile))
				}

				// This is a deferred action that updates the database in the background if needed. Returned errors
				// only work for the non deferred calls.
				_ = UpdateIfNeeded(latest, false, logger, cfg)
			}
		}

		if fileExists(newDatabase) {
			// Make sure this is a valid IP2 database before assigning it to the config
			_, err := GetDatabaseVersion(newDatabase)
			if err != nil {
				logger.Warn(fmt.Sprintf("failed to open database %s: %v", newDatabase, err))
			} else {
				cfg.DatabaseFilePath = newDatabase
			}
		}
	}

	db, err := ip2location.OpenDB(cfg.DatabaseFilePath)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to open database: %w", name, err)
	}

	// Check database version
	version, err := GetDatabaseVersion(cfg.DatabaseFilePath)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to read database version: %w", name, err)
	}
	logger.Debug("using ip2location database version: " + version.String() + " from " + cfg.DatabaseFilePath)

	// Check if database is older than 2 months
	if time.Since(version.Date()) > 60*24*time.Hour {
		bootstrapLogger.Warn("ip2location database is more than 2 months old",
			"version", version.String(),
			"age", time.Since(version.Date()).Round(24*time.Hour))
	}

	allowedIPBlocks, err := initIPBlocks(cfg.AllowedIPBlocks)
	if err != nil {
		return nil, fmt.Errorf("%s: failed loading allowed CIDR blocks: %w", name, err)
	}

	blockedIPBlocks, err := initIPBlocks(cfg.BlockedIPBlocks)
	if err != nil {
		return nil, fmt.Errorf("%s: failed loading blocked CIDR blocks: %w", name, err)
	}

	var banHtmlContent string

	if cfg.BanHtmlFilePath != "" {
		cfg.BanHtmlFilePath = searchFile(cfg.BanHtmlFilePath, "geoblockban.html", bootstrapLogger)
		content, err := os.ReadFile(cfg.BanHtmlFilePath)
		if err != nil {
			return nil, fmt.Errorf("%s: failed to load ban HTML file %s: %w", name, cfg.BanHtmlFilePath, err)
		} else {
			banHtmlContent = string(content)
		}
	}

	// Convert slices to maps for O(1) lookup
	allowedCountries := make(map[string]struct{}, len(cfg.AllowedCountries))
	for _, c := range cfg.AllowedCountries {
		allowedCountries[c] = struct{}{}
	}

	blockedCountries := make(map[string]struct{}, len(cfg.BlockedCountries))
	for _, c := range cfg.BlockedCountries {
		blockedCountries[c] = struct{}{}
	}

	plugin := &Plugin{
		next:                 next,
		name:                 name,
		databaseFile:         cfg.DatabaseFilePath,
		db:                   db,
		enabled:              cfg.Enabled,
		allowedCountries:     allowedCountries,
		blockedCountries:     blockedCountries,
		defaultAllow:         cfg.DefaultAllow,
		allowPrivate:         cfg.AllowPrivate,
		banIfError:           cfg.BanIfError,
		disallowedStatusCode: cfg.DisallowedStatusCode,
		allowedIPBlocks:      allowedIPBlocks,
		blockedIPBlocks:      blockedIPBlocks,
		banHtmlContent:       banHtmlContent,
		bypassHeaders:        cfg.BypassHeaders,
		logger:               logger,
		logBannedRequests:    cfg.LogBannedRequests,
	}

	return plugin, nil
}

// ServeHTTP implements the http.Handler interface.
func (p Plugin) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if !p.enabled {
		p.logger.Debug("plugin disabled, passing request through")
		p.next.ServeHTTP(rw, req)
		return
	}

	// Check for bypass headers
	// Optimize by avoiding multiple map lookups and method calls
	for header, expectedValue := range p.bypassHeaders {
		if actualValue := req.Header.Get(header); actualValue == expectedValue {
			p.logger.Debug("bypassing geoblock due to bypass header match",
				"header", header,
				"value", expectedValue,
				"remote_addr", req.RemoteAddr,
				"x_real_ip", req.Header.Get("x-real-ip"),
				"x_forwarded_for", req.Header.Get("x-forwarded-for"))
			p.next.ServeHTTP(rw, req)
			return
		}
	}

	remoteIPs := p.GetRemoteIPs(req)

	for _, ip := range remoteIPs {
		allowed, country, err := p.CheckAllowed(ip)
		if err != nil {
			var ipChain string = ""
			if len(remoteIPs) > 1 {
				ipChain = strings.Join(remoteIPs, ", ")
			}
			p.logger.Error("request check failed",
				"ip", ip,
				"ip_chain", ipChain,
				"host", req.Host,
				"method", req.Method,
				"path", req.URL.Path,
				"error", err)

			if p.banIfError {
				p.serveBanHtml(rw, ip, "Unknown")
				return
			}
			// keel looping
			continue
		}
		if !allowed {
			var ipChain string = ""
			if len(remoteIPs) > 1 {
				ipChain = strings.Join(remoteIPs, ", ")
			}
			if p.logBannedRequests {
				p.logger.Info("blocked request",
					"ip", ip,
					"ip_chain", ipChain,
					"country", country,
					"host", req.Host,
					"method", req.Method,
					"path", req.URL.Path)
			}
			p.serveBanHtml(rw, ip, country)
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
			ip = cleanIPAddress(ip)
			if ip == "" {
				continue
			}
			uniqIPs[ip] = struct{}{}
		}
	}
	if xri := req.Header.Get("x-real-ip"); xri != "" {
		for _, ip := range strings.Split(xri, ",") {
			ip = cleanIPAddress(ip)
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

func cleanIPAddress(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return ""
	}
	// Split IP from port if port exists (e.g., "192.168.1.1:8080")
	host, _, err := net.SplitHostPort(ip)
	if err == nil {
		return host
	}
	return ip // If no port, return the original IP
}

// CheckAllowed determines if an IP address should be allowed through based on configured rules.
// Returns:
// - allow: whether the request should be allowed
// - country: the detected country code for the IP
// - err: any errors encountered during the check
func (p Plugin) CheckAllowed(ip string) (allow bool, country string, err error) {
	// Discrete IPs have higher priority than countries
	// so deal with them before doing any lookups
	ipAddr := net.ParseIP(ip)
	if ipAddr == nil {
		return false, ip, fmt.Errorf("unable to parse IP address from [%s]", ip)
	}

	// We want this check first because it's fast, even if it makes more sense to have
	// it after the ip whitelist/blacklist verification
	if ipAddr.IsPrivate() {
		return p.allowPrivate, PrivateIpCountryAlias, nil
	}

	blocked, blockedNetworkLength, err := p.isBlockedIPBlocks(ipAddr)
	if err != nil {
		return false, ip, fmt.Errorf("failed to check if IP %q is blocked by IP block: %w", ip, err)
	}

	allowed, allowedNetworkLength, err := p.isAllowedIPBlocks(ipAddr)
	if err != nil {
		return false, ip, fmt.Errorf("failed to check if IP %q is allowed by IP block: %w", ip, err)
	}

	// NB: whichever matched prefix is longer has higher priority: more specific to less specific only if both matched.
	if (allowedNetworkLength < blockedNetworkLength) && (allowedNetworkLength > 0) && (blockedNetworkLength > 0) {
		if blocked {
			return false, country, nil
		}
		if allowed {
			return true, country, nil
		}
	} else {
		if allowed {
			return true, country, nil
		}
		if blocked {
			return false, country, nil
		}
	}

	// Look up the country for this IP
	country, err = p.Lookup(ip)
	if err != nil {
		return false, ip, fmt.Errorf("lookup of %s failed: %w", ip, err)
	}

	// Check if country is in the allowlist (using O(1) map lookup)
	// If found, allow the request immediately
	if _, allowed := p.allowedCountries[country]; allowed {
		return true, country, nil
	}

	// Check if country is in the blocklist (using O(1) map lookup)
	// If found, block the request immediately
	if _, blocked := p.blockedCountries[country]; blocked {
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

	// Avoid redundant assignment and string conversion
	if strings.HasPrefix(strings.ToLower(record.Country_short), "invalid") {
		return "", errors.New(record.Country_short)
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
func (p Plugin) isAllowedIPBlocks(ipAddr net.IP) (bool, int, error) {
	return p.isInIPBlocks(ipAddr, p.allowedIPBlocks)
}

// isBlockedIPBlocks checks if an IP is allowed base on the blocked CIDR blocks
func (p Plugin) isBlockedIPBlocks(ipAddr net.IP) (bool, int, error) {
	return p.isInIPBlocks(ipAddr, p.blockedIPBlocks)
}

// isInIPBlocks indicates whether the given IP exists in any of the IP subnets contained within ipBlocks.
func (p Plugin) isInIPBlocks(ipAddr net.IP, ipBlocks []*net.IPNet) (bool, int, error) {
	for _, block := range ipBlocks {
		if block.Contains(ipAddr) {
			ones, _ := block.Mask.Size()
			return true, ones, nil
		}
	}

	return false, 0, nil
}

// Update the serveBanHtml function to use simple string replacement
func (p Plugin) serveBanHtml(rw http.ResponseWriter, ip, country string) {
	if p.banHtmlContent != "" {
		rw.Header().Set("Content-Type", "text/html; charset=utf-8")
		rw.WriteHeader(p.disallowedStatusCode)

		// Simple string replacements
		content := p.banHtmlContent
		content = strings.ReplaceAll(content, "{{.Country}}", country)
		content = strings.ReplaceAll(content, "{{.IP}}", ip)

		if _, err := rw.Write([]byte(content)); err != nil {
			p.logger.Warn("failed to write ban HTML response", "error", err)
		}
		return
	}
	rw.WriteHeader(p.disallowedStatusCode)
}
