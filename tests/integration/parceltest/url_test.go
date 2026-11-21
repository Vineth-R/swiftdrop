package parceltest

import (
	"net/url"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestParcelTestDatabaseURLIsolation(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{name: "plain URL", query: "sslmode=disable"},
		{name: "libpq database override", query: "sslmode=disable&dbname=shared"},
		{name: "pgx database override", query: "sslmode=disable&database=shared"},
		{name: "both aliases", query: "sslmode=disable&dbname=shared&database=another"},
		{name: "repeated aliases", query: "sslmode=disable&dbname=shared&dbname=another"},
		{name: "uppercase alias", query: "sslmode=disable&DBNAME=shared"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const databaseName = "swiftdrop_parcel_test_0123456789abcdef"
			result, err := isolatedDatabaseURL("postgres://test:test@localhost/shared?"+test.query, databaseName)
			if err != nil {
				t.Fatalf("isolatedDatabaseURL(): %v", err)
			}
			parsed, err := url.Parse(result)
			if err != nil {
				t.Fatalf("parse isolated URL: %v", err)
			}
			if parsed.Path != "/"+databaseName || parsed.Query().Get("sslmode") != "disable" {
				t.Fatal("isolated URL did not preserve connection settings and replace the database")
			}
			config, err := pgx.ParseConfig(result)
			if err != nil || config.Database != databaseName {
				t.Fatalf("pgx selected a different database: %v", err)
			}
		})
	}
}

func TestParcelTestDatabaseURLRejectsInvalidConfiguration(t *testing.T) {
	for _, rawURL := range []string{"", "postgres:///missing-host", "http://localhost/database", "postgres://%"} {
		t.Run(rawURL, func(t *testing.T) {
			if _, err := isolatedDatabaseURL(rawURL, "isolated"); err == nil {
				t.Fatal("invalid test URL was accepted")
			}
		})
	}
}
