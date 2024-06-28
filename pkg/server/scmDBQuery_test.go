package server

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/updatecli/udash/pkg/database"
)

var (
	DatabaseOptions = database.Options{
		URI: "postgres://udash:password@localhost:5432/udash?sslmode=disable",
	}
)

// TestFindTargetSCM tests the FindTargetSCM function.
func TestFindTargetSCM(t *testing.T) {

	err := database.Connect(DatabaseOptions)
	require.NoError(t, err)

	rows, err := dbGetSCMFromTarget()
	require.NoError(t, err)

	t.Logf("rows: %v", rows)

	t.Fail()
}
