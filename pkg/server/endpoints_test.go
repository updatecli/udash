package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/updatecli/udash/pkg/database"
	"github.com/updatecli/udash/test"
)

func TestEndpoints(t *testing.T) {
	eng := newGinEngine(Options{})
	srv := httptest.NewServer(eng)
	defer srv.Close()

	test.SetupDatabase(t)

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

	t.Run("GET /api/pipeline/scms", func(t *testing.T) {
		resp := doGetRequest(t, srv, "/api/pipeline/scms")
		assertJSONResponse(t, resp, map[string]any{
			"scms": []any{},
		}, assert.Equal)

		id, err := database.InsertSCM("https://example.com/testing.git", "main")
		require.NoError(t, err)
		resp = doGetRequest(t, srv, "/api/pipeline/scms")
		assertJSONResponse(t, resp, map[string]any{
			"scms": []any{
				map[string]any{
					"Branch": "main",
					"ID":     id,
					"URL":    "https://example.com/testing.git",
				},
			},
		}, assert.Equal)
	})
}

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

func assertJSONResponse(t *testing.T, res *http.Response, want map[string]any, asserter func(t assert.TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool) {
	t.Helper()

	require.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, "application/json; charset=utf-8", res.Header.Get("Content-Type"))

	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	got := map[string]any{}
	require.NoError(t, json.Unmarshal(b, &got))
	asserter(t, got, want)
}
