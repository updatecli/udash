package test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/updatecli/udash/pkg/database"
)

// SetupDatabase starts PostgreSQL with a "udash" user.
//
// The Migrations are applied to the database.
//
// The running container will be terminated when the test is cleaned up.
func SetupDatabase(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	postgresContainer, err := postgres.Run(ctx,
		"postgres:17@sha256:8d3be35b184e70d81e54cbcbd3df3c0b47f37d06482c0dd1c140db5dbcc6a808",
		postgres.WithDatabase("udash"),
		postgres.WithUsername("udash"),
		postgres.WithPassword("password"),
		postgres.BasicWaitStrategies(),
	)
	t.Cleanup(func() {
		require.NoError(t, testcontainers.TerminateContainer(postgresContainer))
		testcontainers.CleanupContainer(t, postgresContainer)
	})
	t.Log("Postgres Container started")

	dbURL, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	require.NoError(t, database.Connect(database.Options{URI: dbURL}))
	t.Log("Postgres Container connected")
	require.NoError(t, database.RunMigrationUp())
	t.Log("Postgres Container migrations run")
}
