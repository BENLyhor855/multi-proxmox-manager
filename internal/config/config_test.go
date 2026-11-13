package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestConfigurationBoundaries(t *testing.T) {
	t.Setenv("MYSQL_DSN", "user:pass@tcp(127.0.0.1:3306)/mmp")
	t.Setenv("JWT_SECRET", strings.Repeat("s", 48))
	t.Setenv("CREDENTIALS_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	t.Setenv("PVE_ALLOWED_CIDRS", "192.0.2.10/32")
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_ORIGIN", "https://manager.test")
	t.Setenv("HTTP_ADDR", "127.0.0.1:8080")
	t.Setenv("COOKIE_SECURE", "true")
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ k, v string }{{"APP_ORIGIN", "http://manager.test"}, {"HTTP_ADDR", "0.0.0.0:8080"}, {"COOKIE_SECURE", "false"}, {"PVE_ALLOWED_CIDRS", ""}, {"JWT_SECRET", "REPLACE_WITH_AT_LEAST_32_RANDOM_BYTES"}, {"CREDENTIALS_ENCRYPTION_KEY", "bad"}, {"APP_ENV", "prod"}} {
		t.Run(tc.k, func(t *testing.T) {
			t.Setenv(tc.k, tc.v)
			if _, err := Load(); err == nil {
				t.Fatalf("accepted unsafe %s", tc.k)
			}
		})
	}
}
