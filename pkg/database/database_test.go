package database

import (
	"testing"

	"github.com/updatecli/udash/test"
)

func TestDatabase(t *testing.T) {
	// This will fail if the database is not setup correctly
	// It does require a local Docker engine to run.
	test.SetupDatabase(t)
}
