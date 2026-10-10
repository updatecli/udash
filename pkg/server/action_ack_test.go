package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/updatecli/udash/pkg/database"
	"github.com/updatecli/udash/test"
)

// actionAckTestServer serves the acknowledgement routes behind a stubbed identity, wired
// as newGinEngine wires them.
func actionAckTestServer(t *testing.T, identity Principal) *httptest.Server {
	t.Helper()

	gin.SetMode(gin.TestMode)
	r := gin.New()

	group := r.Group("/api/pipeline")
	group.Use(publicReadOnly(fakeIdentityAuth(identity)))
	registerActionAckRoutes(group, true)

	server := httptest.NewServer(r)
	t.Cleanup(server.Close)

	return server
}

func TestActionAcknowledgementAuth(t *testing.T) {
	ctx := context.Background()

	postgresContainer, err := test.SetupDatabase(t, ctx)
	require.NoError(t, err)

	dbURL, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	require.NoError(t, database.Connect(database.Options{URI: dbURL}))
	require.NoError(t, database.RunMigrationUp())

	body := map[string]any{"url": "https://example.com/testing/pull/42", "days": 1, "result": "✔"}

	t.Run("an anonymous visitor cannot acknowledge, even when anybody may read", func(t *testing.T) {
		srv := actionAckTestServer(t, Principal{Subject: "user-viewer", Permission: PermissionViewer})

		status, _ := doJSON(t, http.MethodPut, srv.URL+"/api/pipeline/actions/ack", "", body)
		assert.Equal(t, http.StatusUnauthorized, status)

		status, _ = doJSON(t, http.MethodDelete, srv.URL+"/api/pipeline/actions/ack?url=x", "", nil)
		assert.Equal(t, http.StatusUnauthorized, status)
	})

	t.Run("an identity without any permission cannot acknowledge", func(t *testing.T) {
		srv := actionAckTestServer(t, Principal{Subject: "user-nobody"})

		status, _ := doJSON(t, http.MethodPut, srv.URL+"/api/pipeline/actions/ack", "session", body)
		assert.Equal(t, http.StatusForbidden, status)
	})

	t.Run("a viewer acknowledges under their name", func(t *testing.T) {
		t.Cleanup(func() {
			_, err := database.DB.Exec(ctx, "DELETE FROM action_acks")
			require.NoError(t, err)
		})

		srv := actionAckTestServer(t, Principal{Subject: "user-viewer", Name: "Pat", Permission: PermissionViewer})

		status, _ := doJSON(t, http.MethodPut, srv.URL+"/api/pipeline/actions/ack", "session", body)
		require.Equal(t, http.StatusNoContent, status)

		by := ""
		require.NoError(t, database.DB.QueryRow(ctx, "SELECT acknowledged_by FROM action_acks").Scan(&by))
		assert.Equal(t, "Pat", by)

		status, _ = doJSON(t, http.MethodDelete, srv.URL+"/api/pipeline/actions/ack?url="+"https%3A%2F%2Fexample.com%2Ftesting%2Fpull%2F42", "session", nil)
		require.Equal(t, http.StatusNoContent, status)

		count := 0
		require.NoError(t, database.DB.QueryRow(ctx, "SELECT count(*) FROM action_acks").Scan(&count))
		assert.Zero(t, count)
	})
}
