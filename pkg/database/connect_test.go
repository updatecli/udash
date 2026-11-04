package database

import (
	"context"
	"testing"
	"time"

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
