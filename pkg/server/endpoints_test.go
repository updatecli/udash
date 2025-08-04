package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/updatecli/udash/pkg/database"
	"github.com/updatecli/udash/test"
	"github.com/updatecli/updatecli/pkg/core/reports"
)

func TestEndpoints(t *testing.T) {
	eng := newGinEngine(Options{})
	srv := httptest.NewServer(eng)
	defer srv.Close()

	ctx := context.Background()

	postgresContainer, err := test.SetupDatabase(t, ctx)
	require.NoError(t, err)

	dbURL, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	// Connect to the database and run migrations
	require.NoError(t, database.Connect(database.Options{URI: dbURL}))
	t.Log("Postgres Container connected")
	require.NoError(t, database.RunMigrationUp())
	t.Log("Postgres Container migrations run")

	t.Run("GET /api", func(t *testing.T) {
		resp := doGetRequest(t, srv, "/api")
		assertJSONResponse(t, resp, map[string]any{
			"message": "Welcome to the Udash API",
		}, assert.Equal)
	})

	t.Run("GET /api/ping", func(t *testing.T) {
		resp := doGetRequest(t, srv, "/api/ping")
		assertJSONResponse(t, resp, map[string]any{
			"message": "pong",
		}, assert.Equal)
	})

	t.Run("GET /api/about", func(t *testing.T) {
		resp := doGetRequest(t, srv, "/api/about")
		assertJSONResponse(t, resp, map[string]any{
			"version": map[string]any{},
		}, assert.Equal)
	})

	// TODO: Test query parameters:
	// scmid, url, branch, summary
	t.Run("GET /api/pipeline/scms", func(t *testing.T) {
		resp := doGetRequest(t, srv, "/api/pipeline/scms")
		assertJSONResponse(t, resp, map[string]any{
			"scms": []any{},
		}, assert.Equal)

		id, err := database.InsertSCM(context.TODO(), "https://example.com/testing.git", "main")
		require.NoError(t, err)
		resp = doGetRequest(t, srv, "/api/pipeline/scms")
		assertJSONResponse(t, resp, []map[string]any{
			{
				"Branch": "main",
				"ID":     id,
				"URL":    "https://example.com/testing.git",
			},
		}, removeFieldsAsserter("scms", "Created_at", "Updated_at"))
	})

	// TODO: Test query parameters:
	// scmid
	t.Run("GET /api/pipeline/reports", func(t *testing.T) {
		resp := doGetRequest(t, srv, "/api/pipeline/reports")
		assertJSONResponse(t, resp, map[string]any{
			"data": []any{},
		}, assert.Equal)

		reportID, err := database.InsertReport(context.TODO(), reports.Report{
			Name:       "ci: bump Venom version",
			Result:     "✔",
			ID:         "1de1797bbc925e08e473178425b11eb16fc547291f4b45274da24c2b00e2afc3",
			PipelineID: "venom",
			Actions: map[string]*reports.Action{
				"default": {
					ID: "44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a",
				},
			},
		})
		require.NoError(t, err)

		resp = doGetRequest(t, srv, "/api/pipeline/reports")
		assertJSONResponse(t, resp, []map[string]any{
			{
				"ID":     reportID,
				"Name":   "ci: bump Venom version",
				"Result": "✔",
				"Report": map[string]any{
					"Name":       "ci: bump Venom version",
					"Err":        "",
					"Result":     "✔",
					"ID":         "1de1797bbc925e08e473178425b11eb16fc547291f4b45274da24c2b00e2afc3",
					"PipelineID": "venom",
					"Actions": map[string]any{
						"default": map[string]any{
							"id": "44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a",
						},
					},
					"Sources":    nil,
					"Conditions": nil,
					"Targets":    nil,
					"ReportURL":  "",
				},
				"FilteredResourceID": "",
			},
		}, removeFieldsAsserter("data", "CreatedAt", "UpdatedAt"))
	})
}

// Disabling unparam linter as I am planning to extend the test to use the ops
//
//nolint:unparam
func doGetRequest(t *testing.T, ts *httptest.Server, path string, opts ...func(*http.Request)) *http.Response {
	t.Helper()
	r, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s%s", ts.URL, path), nil)
	require.NoError(t, err)

	for _, o := range opts {
		o(r)
	}

	resp, err := ts.Client().Do(r)
	require.NoError(t, err)

	return resp
}

type assertionFunc func(t assert.TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool

func assertJSONResponse(t *testing.T, res *http.Response, want any, asserter assertionFunc) {
	t.Helper()

	require.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, "application/json; charset=utf-8", res.Header.Get("Content-Type"))

	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	got := map[string]any{}
	require.NoError(t, json.Unmarshal(b, &got))

	asserter(t, want, got)
}

func deleteKeys(source map[string]any, keys ...string) map[string]any {
	updated := maps.Clone(source)
	for _, key := range keys {
		delete(updated, key)
	}

	return updated
}

func removeFieldsAsserter(key string, fields ...string) assertionFunc {
	return func(t assert.TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool {
		blob := actual.(map[string]any)

		var cleaned []map[string]any
		for _, data := range blob[key].([]any) {
			cleaned = append(cleaned, deleteKeys(data.(map[string]any), fields...))
		}
		return assert.Equal(t, expected, cleaned)
	}
}
