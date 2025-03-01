package traefik_plugin_geoblock

import (
	"testing"
	"time"
)

func TestGetDateFromName(t *testing.T) {
	tests := []struct {
		name    string
		dbPath  string
		want    time.Time
		wantErr bool
	}{
		{
			name:    "valid filename",
			dbPath:  "20240315_IP2LOCATION-LITE-DB1.BIN",
			want:    time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
			wantErr: false,
		},
		{
			name:    "valid filename with path",
			dbPath:  "/path/to/20240315_IP2LOCATION-LITE-DB1.BIN",
			want:    time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
			wantErr: false,
		},
		{
			name:    "valid filename with windows path",
			dbPath:  `C:\path\to\20240315_IP2LOCATION-LITE-DB1.BIN`,
			want:    time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
			wantErr: false,
		},
		{
			name:    "invalid date format",
			dbPath:  "invalid_IP2LOCATION-LITE-DB1.BIN",
			want:    time.Time{},
			wantErr: true,
		},
		{
			name:    "empty string",
			dbPath:  "",
			want:    time.Time{},
			wantErr: true,
		},
		{
			name:    "invalid year",
			dbPath:  "abcd0315_IP2LOCATION-LITE-DB1.BIN",
			want:    time.Time{},
			wantErr: true,
		},
		{
			name:    "invalid month",
			dbPath:  "2024xx15_IP2LOCATION-LITE-DB1.BIN",
			want:    time.Time{},
			wantErr: true,
		},
		{
			name:    "invalid day",
			dbPath:  "202403xx_IP2LOCATION-LITE-DB1.BIN",
			want:    time.Time{},
			wantErr: true,
		},
		{
			name:    "too short date",
			dbPath:  "2024_IP2LOCATION-LITE-DB1.BIN",
			want:    time.Time{},
			wantErr: true,
		},
		{
			name:    "no underscore",
			dbPath:  "20240315IP2LOCATION-LITE-DB1.BIN",
			want:    time.Time{},
			wantErr: true,
		},
		{
			name:    "leap year date",
			dbPath:  "20240229_IP2LOCATION-LITE-DB1.BIN",
			want:    time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC),
			wantErr: false,
		},
		{
			name:    "last day of year",
			dbPath:  "20241231_IP2LOCATION-LITE-DB1.BIN",
			want:    time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetDateFromName(tt.dbPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetDateFromName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !got.Equal(tt.want) {
				t.Errorf("GetDateFromName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetDBVersion(t *testing.T) {
	// Test successful case
	version, err := GetDatabaseVersion(dbFilePath)
	if err != nil {
		t.Errorf("Expected no error for valid database, got: %v", err)
	}
	if version.Month != 2 {
		t.Errorf("Expected month 5, got: %d", version.Month)
	}

	// Test error case
	version, err = GetDatabaseVersion("20240315_IP2LOCATION-LITE-INVALID.BIN")
	if err == nil {
		t.Error("Expected error for invalid database, got nil")
	}
	if version != nil {
		t.Error("Expected nil version for invalid database")
	}
}
