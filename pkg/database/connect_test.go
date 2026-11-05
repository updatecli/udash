package database

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unreachableURI points at a port nothing listens on.
const unreachableURI = "postgres://udash@127.0.0.1:1/udash?sslmode=disable&connect_timeout=1"

func TestConnectRetriesUntilTimeout(t *testing.T) {
	start := time.Now()
	err := Connect(context.Background(), Options{URI: unreachableURI, ConnectTimeout: 2 * time.Second})

	require.ErrorContains(t, err, "database not ready after 2s")
	assert.GreaterOrEqual(t, time.Since(start), 2*time.Second)
}

func TestConnectStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	require.Error(t, Connect(ctx, Options{URI: unreachableURI, ConnectTimeout: time.Minute}))
	assert.Less(t, time.Since(start), 5*time.Second)
}

func TestIsPermanent(t *testing.T) {
	authFailed := &pgconn.PgError{Code: "28P01"}
	missingDB := &pgconn.PgError{Code: "3D000"}
	refused := errors.New("dial tcp 127.0.0.1:5432: connect: connection refused")
	tooMany := &pgconn.PgError{Code: "53300"}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"network error", refused, false},
		{"transient PostgreSQL error", tooMany, false},
		{"wrong password", authFailed, true},
		{"wrapped missing database", fmt.Errorf("connect: %w", missingDB), true},
		{"every host permanent", fmt.Errorf("connect: %w", errors.Join(authFailed, missingDB)), true},
		{"one host down", fmt.Errorf("connect: %w", errors.Join(refused, authFailed)), false},
		{"one host transient PostgreSQL error", errors.Join(authFailed, tooMany), false},
		{"empty join", errors.Join(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isPermanent(tt.err))
		})
	}
}
