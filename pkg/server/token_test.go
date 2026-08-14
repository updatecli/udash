package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/updatecli/udash/pkg/database"
	"github.com/updatecli/udash/test"
	"github.com/updatecli/updatecli/pkg/core/reports"
	"github.com/updatecli/updatecli/pkg/core/result"
)

// fakeIdentityAuth stands in for the identity provider middleware, so the token
// endpoints can be tested without a live Zitadel.
func fakeIdentityAuth(principal Principal) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{errMessageType: ErrUnauthenticated})
			return
		}
		setPrincipal(c, principal)
		c.Next()
	}
}

// tokenTestServer wires the token endpoints behind a stubbed identity, plus the
// report write route so scope enforcement can be checked end to end.
func tokenTestServer(t *testing.T, identity Principal) *httptest.Server {
	t.Helper()

	gin.SetMode(gin.TestMode)
	r := gin.New()

	auth := udashTokenAuth(snapshotResolver{}, fakeIdentityAuth(identity))
	registerTokenRoutes(r, auth)

	r.POST("/api/pipeline/reports", auth, requireScope(ScopeReportsWrite), CreatePipelineReport)

	server := httptest.NewServer(r)
	t.Cleanup(server.Close)

	return server
}

func doJSON(t *testing.T, method, url, bearer string, body any) (int, map[string]any) {
	t.Helper()

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequest(method, url, reader)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	decoded := map[string]any{}
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	if len(raw) > 0 && raw[0] == '{' {
		require.NoError(t, json.Unmarshal(raw, &decoded))
	}

	return resp.StatusCode, decoded
}

func TestAPITokens(t *testing.T) {
	ctx := context.Background()

	postgresContainer, err := test.SetupDatabase(t, ctx)
	require.NoError(t, err)

	dbURL, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	require.NoError(t, database.Connect(database.Options{URI: dbURL}))
	require.NoError(t, database.RunMigrationUp())

	publisher := Principal{Subject: "user-publisher", Name: "Pat", Permission: PermissionPublisher}

	t.Run("lifecycle", func(t *testing.T) {
		srv := tokenTestServer(t, publisher)

		status, created := doJSON(t, http.MethodPost, srv.URL+"/api/tokens", "session", map[string]any{
			"name": "ci",
		})
		require.Equal(t, http.StatusCreated, status)

		token, _ := created["token"].(string)
		require.NotEmpty(t, token)
		assert.True(t, len(token) > len(APITokenPrefix), "the token must carry the prefix and some entropy")
		assert.Contains(t, token, APITokenPrefix)
		assert.Nil(t, created["expires_at"], "a token created without an expiry never expires")

		// The token authenticates on its own, with no identity provider involved.
		status, who := doJSON(t, http.MethodGet, srv.URL+"/api/whoami", token, nil)
		require.Equal(t, http.StatusOK, status)
		assert.Equal(t, "user-publisher", who["subject"])
		assert.Equal(t, "ci", who["tokenName"])

		// And it may publish.
		status, _ = doJSON(t, http.MethodPost, srv.URL+"/api/pipeline/reports", token, map[string]any{
			"Name": "ci: bump something", "ID": "abc", "PipelineID": "p",
		})
		assert.Equal(t, http.StatusCreated, status)

		id, _ := created["id"].(string)
		require.NotEmpty(t, id)

		status, _ = doJSON(t, http.MethodDelete, srv.URL+"/api/tokens/"+id, "session", nil)
		require.Equal(t, http.StatusOK, status)

		// Once revoked it stops working.
		status, _ = doJSON(t, http.MethodGet, srv.URL+"/api/whoami", token, nil)
		assert.Equal(t, http.StatusUnauthorized, status)
	})

	t.Run("a token cannot mint another token", func(t *testing.T) {
		srv := tokenTestServer(t, publisher)

		status, created := doJSON(t, http.MethodPost, srv.URL+"/api/tokens", "session", map[string]any{"name": "ci"})
		require.Equal(t, http.StatusCreated, status)
		token := created["token"].(string)

		// Otherwise a leaked token could issue fresh ones and outlive its revocation.
		status, _ = doJSON(t, http.MethodPost, srv.URL+"/api/tokens", token, map[string]any{"name": "sneaky"})
		assert.Equal(t, http.StatusForbidden, status)
	})

	t.Run("a viewer cannot create a token", func(t *testing.T) {
		srv := tokenTestServer(t, Principal{Subject: "user-viewer", Permission: PermissionViewer})

		// This is what stops everybody who can sign in from minting a publishing token.
		status, _ := doJSON(t, http.MethodPost, srv.URL+"/api/tokens", "session", map[string]any{"name": "nope"})
		assert.Equal(t, http.StatusForbidden, status)
	})

	t.Run("a token cannot be granted more than its creator", func(t *testing.T) {
		srv := tokenTestServer(t, Principal{Subject: "user-viewer-2", Permission: PermissionViewer})

		status, _ := doJSON(t, http.MethodPost, srv.URL+"/api/tokens", "session", map[string]any{
			"name":   "escalate",
			"scopes": []string{ScopeReportsWrite},
		})
		assert.Equal(t, http.StatusForbidden, status)
	})

	t.Run("a read only token cannot publish", func(t *testing.T) {
		srv := tokenTestServer(t, publisher)

		status, created := doJSON(t, http.MethodPost, srv.URL+"/api/tokens", "session", map[string]any{
			"name":   "read-only",
			"scopes": []string{ScopeReportsRead},
		})
		require.Equal(t, http.StatusCreated, status)
		token := created["token"].(string)

		status, _ = doJSON(t, http.MethodPost, srv.URL+"/api/pipeline/reports", token, map[string]any{
			"Name": "nope", "ID": "def", "PipelineID": "p",
		})
		assert.Equal(t, http.StatusForbidden, status)
	})

	t.Run("an expired token is rejected", func(t *testing.T) {
		expired := time.Now().Add(-time.Hour)
		token, hash, err := generateAPIToken()
		require.NoError(t, err)

		_, err = database.InsertAPIToken(ctx, "expired", "user-publisher",
			string(PermissionPublisher), []string{ScopeReportsWrite}, hash, &expired)
		require.NoError(t, err)

		srv := tokenTestServer(t, publisher)
		status, _ := doJSON(t, http.MethodGet, srv.URL+"/api/whoami", token, nil)
		assert.Equal(t, http.StatusUnauthorized, status)
	})

	t.Run("an unknown token is rejected", func(t *testing.T) {
		srv := tokenTestServer(t, publisher)

		status, _ := doJSON(t, http.MethodGet, srv.URL+"/api/whoami", APITokenPrefix+"nonexistent", nil)
		assert.Equal(t, http.StatusUnauthorized, status)
	})

	t.Run("one identity cannot revoke another's token", func(t *testing.T) {
		owner := tokenTestServer(t, publisher)

		status, created := doJSON(t, http.MethodPost, owner.URL+"/api/tokens", "session", map[string]any{"name": "mine"})
		require.Equal(t, http.StatusCreated, status)
		id := created["id"].(string)

		other := tokenTestServer(t, Principal{Subject: "somebody-else", Permission: PermissionPublisher})
		status, _ = doJSON(t, http.MethodDelete, other.URL+"/api/tokens/"+id, "session", nil)
		assert.Equal(t, http.StatusNotFound, status)

		// An administrator may.
		admin := tokenTestServer(t, Principal{Subject: "an-admin", Permission: PermissionAdmin})
		status, _ = doJSON(t, http.MethodDelete, admin.URL+"/api/tokens/"+id, "session", nil)
		assert.Equal(t, http.StatusOK, status)
	})

	t.Run("listing is scoped to the caller unless they are an administrator", func(t *testing.T) {
		srv := tokenTestServer(t, Principal{Subject: "user-lister", Permission: PermissionPublisher})

		status, _ := doJSON(t, http.MethodPost, srv.URL+"/api/tokens", "session", map[string]any{"name": "mine"})
		require.Equal(t, http.StatusCreated, status)

		req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/tokens", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer session")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		listed := []map[string]any{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&listed))
		require.NotEmpty(t, listed)
		for _, entry := range listed {
			assert.Equal(t, "user-lister", entry["subject"])
			assert.NotContains(t, entry, "token", "the secret must never be listed")
		}

		// Asking for everybody's is refused to a non administrator.
		status, _ = doJSON(t, http.MethodGet, srv.URL+"/api/tokens?all=true", "session", nil)
		assert.Equal(t, http.StatusForbidden, status)
	})

	t.Run("unauthenticated requests are refused", func(t *testing.T) {
		srv := tokenTestServer(t, publisher)

		status, _ := doJSON(t, http.MethodGet, srv.URL+"/api/tokens", "", nil)
		assert.Equal(t, http.StatusUnauthorized, status)
	})
}

func TestReportAttribution(t *testing.T) {
	ctx := context.Background()

	postgresContainer, err := test.SetupDatabase(t, ctx)
	require.NoError(t, err)

	dbURL, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	require.NoError(t, database.Connect(database.Options{URI: dbURL}))
	require.NoError(t, database.RunMigrationUp())

	publisher := Principal{Subject: "user-publisher", Permission: PermissionPublisher}
	srv := tokenTestServer(t, publisher)

	status, created := doJSON(t, http.MethodPost, srv.URL+"/api/tokens", "session", map[string]any{"name": "ci"})
	require.Equal(t, http.StatusCreated, status)
	token := created["token"].(string)
	tokenID := created["id"].(string)

	status, published := doJSON(t, http.MethodPost, srv.URL+"/api/pipeline/reports", token, map[string]any{
		"Name": "ci: attributed", "ID": "attributed", "PipelineID": "p",
	})
	require.Equal(t, http.StatusCreated, status)

	reportID, _ := published["reportid"].(string)
	require.NotEmpty(t, reportID)

	var subject, storedTokenID *string
	require.NoError(t, database.DB.QueryRow(ctx,
		"SELECT created_by_subject, created_by_token_id::text FROM pipelineReports WHERE id = $1",
		reportID,
	).Scan(&subject, &storedTokenID))

	require.NotNil(t, subject)
	assert.Equal(t, "user-publisher", *subject)
	require.NotNil(t, storedTokenID)
	assert.Equal(t, tokenID, *storedTokenID)
}

func TestReportAttributionWithoutAuth(t *testing.T) {
	ctx := context.Background()

	postgresContainer, err := test.SetupDatabase(t, ctx)
	require.NoError(t, err)

	dbURL, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	require.NoError(t, database.Connect(database.Options{URI: dbURL}))
	require.NoError(t, database.RunMigrationUp())

	// An instance running without authentication has nobody to attribute to, and
	// must keep publishing regardless.
	reportID, err := database.InsertReport(ctx, anonymousReport(), database.Publisher{})
	require.NoError(t, err)

	var subject, tokenID *string
	require.NoError(t, database.DB.QueryRow(ctx,
		"SELECT created_by_subject, created_by_token_id::text FROM pipelineReports WHERE id = $1",
		reportID,
	).Scan(&subject, &tokenID))

	assert.Nil(t, subject)
	assert.Nil(t, tokenID)
}

// anonymousReport is a minimal report, for the attribution tests.
func anonymousReport() reports.Report {
	return reports.Report{
		Name:       "ci: anonymous",
		Result:     result.SUCCESS,
		ID:         "anonymous",
		PipelineID: "p",
	}
}
