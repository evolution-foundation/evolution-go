package config

import (
	"database/sql"
	"slices"
	"testing"
)

// The only guard for the lib/pq import: build and vet pass without it.
func TestPostgresDriverRegistered(t *testing.T) {
	if !slices.Contains(sql.Drivers(), "postgres") {
		t.Fatalf("driver \"postgres\" not registered; sql.Drivers() = %v", sql.Drivers())
	}
}
