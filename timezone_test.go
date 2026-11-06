package postgres

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestTimeZoneParsing(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want string
	}{
		{
			name: "password containing TimeZone= should not be parsed as timezone",
			dsn:  "host=localhost user=test password=TimeZone=secret dbname=testdb",
			want: "",
		},
		{
			name: "valid TimeZone parameter should be parsed",
			dsn:  "host=localhost user=test password=secret dbname=testdb TimeZone=Asia/Jakarta",
			want: "Asia/Jakarta",
		},
		{
			name: "valid timezone parameter (lowercase) should be parsed",
			dsn:  "host=localhost user=test password=secret dbname=testdb timezone=Asia/Jakarta",
			want: "Asia/Jakarta",
		},
		{
			name: "valid time_zone parameter should be parsed",
			dsn:  "host=localhost user=test password=secret dbname=testdb time_zone=Asia/Jakarta",
			want: "Asia/Jakarta",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := pgx.ParseConfig(tt.dsn)
			if err != nil {
				t.Fatalf("pgx.ParseConfig failed: %v", err)
			}

			var tz string
			for key, val := range config.RuntimeParams {
				lowerKey := strings.ToLower(key)
				if lowerKey == "timezone" || lowerKey == "time_zone" {
					tz = val
					break
				}
			}

			if tz != tt.want {
				t.Errorf("Expected timezone %q, but got %q", tt.want, tz)
			}
		})
	}
}
