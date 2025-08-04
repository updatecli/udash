package test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// SetupDatabase starts PostgreSQL with a "udash" user.
//
// The Migrations are applied to the database.
//
// The running container will be terminated when the test is cleaned up.
func SetupDatabase(t *testing.T, ctx context.Context) (*postgres.PostgresContainer, error) {
	t.Helper()

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

	return postgresContainer, err
}
