package database

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/updatecli/udash/test"
)

func TestDatabase(t *testing.T) {

	ctx := context.Background()

	// This will fail if the database is not setup correctly
	// It does require a local Docker engine to run.
	postgresContainer, err := test.SetupDatabase(t, ctx)
	assert.NoError(t, err, "Failed to setup the database")

	dbURL, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	require.NoError(t, Connect(Options{URI: dbURL}))
	t.Log("Postgres Container connected")
	require.NoError(t, RunMigrationUp())
	t.Log("Postgres Container migrations run")
}
