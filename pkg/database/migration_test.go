package database

import (
	"context"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/stretchr/testify/require"
	"github.com/updatecli/udash/test"
)

// firstReversibleVersion is where this test starts rolling back from.
//
// Everything from here up must be reversible. Going further down does not work
// today: migration 000003 recreates its index with jsonb_path_ops while the
// column it migrates back to is json, so postgres rejects it. That predates the
// migrations this test covers and is left alone.
const firstReversibleVersion = 11

// TestMigrationsAreReversible walks the recent migrations down and back up.
//
// A migration which cannot be undone is only discovered when a rollback is
// needed, which is the worst moment to find out.
func TestMigrationsAreReversible(t *testing.T) {
	ctx := context.Background()

	postgresContainer, err := test.SetupDatabase(t, ctx)
	require.NoError(t, err)

	dbURL, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	require.NoError(t, Connect(Options{URI: dbURL}))

	source, err := iofs.New(fs, "migrations")
	require.NoError(t, err)

	m, err := migrate.NewWithSourceInstance("iofs", source, URI)
	require.NoError(t, err)

	require.NoError(t, m.Up())
	require.NoError(t, m.Migrate(firstReversibleVersion))
	require.NoError(t, m.Up())

	// The tables the latest migrations add must be back.
	for _, table := range []string{"api_tokens", "pipelinereports"} {
		var exists bool
		require.NoError(t, DB.QueryRow(ctx,
			"SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = $1)", table,
		).Scan(&exists))
		require.True(t, exists, "table %q must exist after migrating back up", table)
	}

	var count int
	require.NoError(t, DB.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.columns
		 WHERE table_name = 'pipelinereports'
		   AND column_name IN ('created_by_subject', 'created_by_token_id')`,
	).Scan(&count))
	require.Equal(t, 2, count, "the attribution columns must exist after migrating back up")
}
