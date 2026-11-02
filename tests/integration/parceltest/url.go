package parceltest

import (
	"errors"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
)

func isolatedDatabaseURL(rawURL, databaseName string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", errors.New("invalid PostgreSQL test URL")
	}
	validScheme := parsed.Scheme == "postgres" || parsed.Scheme == "postgresql"
	if !validScheme || parsed.Host == "" {
		return "", errors.New("PostgreSQL test URL must include a scheme and host")
	}
	parsed.Path = "/" + databaseName
	parsed.RawPath = ""
	query := parsed.Query()
	for key := range query {
		// Both pgx and migration-driver aliases must not override the generated
		// database path and send destructive rollback tests to the source database.
		if strings.EqualFold(key, "dbname") || strings.EqualFold(key, "database") {
			query.Del(key)
		}
	}
	parsed.RawQuery = query.Encode()
	result := parsed.String()
	config, err := pgx.ParseConfig(result)
	if err != nil || config.Database != databaseName {
		return "", errors.New("PostgreSQL test URL does not select the generated database")
	}
	return result, nil
}
