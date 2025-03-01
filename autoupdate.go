package traefik_plugin_geoblock

import (
	"archive/zip"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	liteDownloadURL  = "https://download.ip2location.com/lite/IP2LOCATION-LITE-DB1.IPV6.BIN.ZIP"
	tokenDownloadURL = "https://www.ip2location.com/download?token=%s&file=%s"
)

// UpdateIfNeeded checks if the database needs updating and performs the update if necessary.
// If runSync is true, the update will be performed synchronously, otherwise it runs in background.
func UpdateIfNeeded(dbPath string, runSync bool, logger *slog.Logger, config *Config) error {
	var performUpdate bool
	if dbPath == "" {
		// Empty path means we need to update
		logger.Info("no database path provided, update needed")
		performUpdate = true
	} else {
		dbDate, err := GetDateFromName(dbPath)
		if err != nil {
			logger.Warn("cannot determine database age", "error", err)
			performUpdate = true
		} else if time.Since(dbDate) > 30*24*time.Hour {
			// Database is older than a month, update
			logger.Info("database is older than 30 days, updating", "sync", runSync)
			performUpdate = true
		}
	}

	if !performUpdate {
		return nil
	}

	if runSync {
		return downloadAndUpdateDatabase(config, logger)
	}

	// Run update asynchronously
	go func() {
		if err := downloadAndUpdateDatabase(config, logger); err != nil {
			logger.Error("async database update failed", "error", err)
		}
	}()
	logger.Info("database update started asynchronously")
	return nil
}

// findLatestDatabase finds the most recent database file in the specified directory
func findLatestDatabase(dir string, dbCode string) (string, error) {
	if dbCode == "" {
		dbCode = "DB1"
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	files, err := filepath.Glob(filepath.Join(dir, fmt.Sprintf("*IP2LOCATION-LITE-%s.IPV6.BIN", dbCode)))
	if err != nil {
		return "", err
	}

	var latest string
	var latestDate time.Time

	for _, f := range files {
		date, err := GetDateFromName(f)
		if err != nil {
			continue
		}

		if latest == "" || date.After(latestDate) {
			latest = f
			latestDate = date
		}
	}

	return latest, nil
}

func downloadAndUpdateDatabase(cfg *Config, logger *slog.Logger) error {
	dbCode := cfg.DatabaseAutoUpdateCode
	if dbCode == "" {
		dbCode = "DB1"
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(cfg.DatabaseAutoUpdateDir, 0755); err != nil {
		return fmt.Errorf("failed to create database directory: %w", err)
	}

	// Create lock file
	lockFile := filepath.Join(cfg.DatabaseAutoUpdateDir, "update.lock")

	// Check if lock file exists and its age
	if fi, err := os.Stat(lockFile); err == nil {
		if time.Since(fi.ModTime()) < time.Hour {
			return fmt.Errorf("another update is in progress and is probably stuck (lock file: %s)", lockFile)
		}
		// Lock is older than 1 hour, consider it stale
		os.Remove(lockFile)
	}

	// Create lock file
	lock, err := os.Create(lockFile)
	if err != nil {
		return fmt.Errorf("failed to create lock file: %w", err)
	}
	defer func() {
		lock.Close()
		os.Remove(lockFile)
	}()

	// Create temporary directory
	tmpDir, err := os.MkdirTemp(cfg.DatabaseAutoUpdateDir, "ip2location-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Download database
	var downloadURL string
	if cfg.DatabaseAutoUpdateToken != "" {
		downloadURL = fmt.Sprintf(tokenDownloadURL, cfg.DatabaseAutoUpdateToken,
			fmt.Sprintf("IP2LOCATION-LITE-%s.IPV6.BIN.ZIP", dbCode))
	} else {
		downloadURL = liteDownloadURL
	}

	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %s", resp.Status)
	}

	// Save and process zip file
	zipPath := filepath.Join(tmpDir, "database.zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return fmt.Errorf("failed to create zip file: %w", err)
	}

	if _, err := io.Copy(zipFile, resp.Body); err != nil {
		zipFile.Close()
		return fmt.Errorf("failed to save zip file: %w", err)
	}
	zipFile.Close()

	// Extract database file
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip file: %w", err)
	}
	defer reader.Close()

	var dbFile *os.File
	for _, file := range reader.File {
		if filepath.Ext(file.Name) == ".BIN" {
			rc, err := file.Open()
			if err != nil {
				return fmt.Errorf("failed to open zip entry: %w", err)
			}

			dbFile, err = os.Create(filepath.Join(tmpDir, "database.bin"))
			if err != nil {
				rc.Close()
				return fmt.Errorf("failed to create database file: %w", err)
			}

			_, err = io.Copy(dbFile, rc)
			rc.Close()
			if err != nil {
				dbFile.Close()
				return fmt.Errorf("failed to extract database: %w", err)
			}
			break
		}
	}

	if dbFile == nil {
		return fmt.Errorf("no database file found in archive")
	}
	dbFile.Close()

	// Verify database and get version for naming
	tmpDBPath := filepath.Join(tmpDir, "database.bin")
	version, err := GetDatabaseVersion(tmpDBPath)
	if err != nil {
		return fmt.Errorf("invalid database file: %w", err)
	}

	// Use database version date for the filename
	finalName := fmt.Sprintf("%s_IP2LOCATION-LITE-%s.IPV6.BIN", version.Date().Format("20060102"), dbCode)
	finalPath := filepath.Join(cfg.DatabaseAutoUpdateDir, finalName)

	if err := copyFile(tmpDBPath, finalPath, false); err != nil {
		return fmt.Errorf("failed to copy database to final location: %w", err)
	}

	logger.Info("database updated successfully" + finalPath)
	return nil
}
