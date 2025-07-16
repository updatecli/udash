package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEndpoints(t *testing.T) {
	eng := newGinEngine(Options{})
	srv := httptest.NewServer(eng)
	defer srv.Close()

	t.Run("GET /api", func(t *testing.T) {
		resp := getRequest(t, srv, "/api")
		assertJSONResponse(t, resp, map[string]any{
			"message": "Welcome to the Udash API",
		})
	})

	t.Run("GET /api/ping", func(t *testing.T) {
		resp := getRequest(t, srv, "/api/ping")
		assertJSONResponse(t, resp, map[string]any{
			"message": "pong",
		})
	})

	t.Run("GET /api/about", func(t *testing.T) {
		resp := getRequest(t, srv, "/api/about")
		assertJSONResponse(t, resp, map[string]any{
			"version": map[string]any{},
		})
	})

	t.Run("GET /api/pipeline/scms", func(t *testing.T) {
		resp := getRequest(t, srv, "/api/pipeline/scms")
		assertJSONResponse(t, resp, map[string]any{
			"version": map[string]any{},
		})
	})
}

func getRequest(t *testing.T, ts *httptest.Server, path string, opts ...func(*http.Request)) *http.Response {
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

func assertJSONResponse(t *testing.T, res *http.Response, want map[string]any) {
	t.Helper()

	require.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, "application/json; charset=utf-8", res.Header.Get("Content-Type"))

	defer res.Body.Close()
	b, err := ioutil.ReadAll(res.Body)
	require.NoError(t, err)
	got := map[string]any{}
	err = json.Unmarshal(b, &got)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
