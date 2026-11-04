package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAPIRouteGuards checks which routes require authentication for each auth setup.
// The route table must list every registered route, so a new one cannot ship unguarded.
func TestAPIRouteGuards(t *testing.T) {
	gin.SetMode(gin.TestMode)

	type route struct {
		method, path string
		class        string
	}

	routes := []route{
		{http.MethodGet, "/pipeline/labels", "read"},
		{http.MethodGet, "/pipeline/scms", "read"},
		{http.MethodGet, "/pipeline/reports", "read"},
		{http.MethodGet, "/pipeline/reports/:id", "read"},
		{http.MethodGet, "/pipeline/config/kinds", "read"},
		{http.MethodGet, "/pipeline/config/sources", "read"},
		{http.MethodGet, "/pipeline/config/conditions", "read"},
		{http.MethodGet, "/pipeline/config/targets", "read"},
		{http.MethodPost, "/pipeline/config/sources/search", "read"},
		{http.MethodPost, "/pipeline/config/conditions/search", "read"},
		{http.MethodPost, "/pipeline/config/targets/search", "read"},
		{http.MethodPost, "/pipeline/labels/search", "read"},
		{http.MethodPost, "/pipeline/reports/search", "read"},
		{http.MethodPost, "/pipeline/reports/summary", "read"},
		{http.MethodPost, "/pipeline/actions/search", "read"},
		{http.MethodPost, "/pipeline/scms/search", "read"},
		{http.MethodPut, "/pipeline/actions/ack", "ack"},
		{http.MethodDelete, "/pipeline/actions/ack", "ack"},
		{http.MethodPost, "/pipeline/reports", "write"},
		{http.MethodPut, "/pipeline/reports/:id", "write"},
		{http.MethodDelete, "/pipeline/reports/:id", "write"},
		{http.MethodPost, "/tokens", "token"},
		{http.MethodGet, "/tokens", "token"},
		{http.MethodDelete, "/tokens/:id", "token"},
		{http.MethodDelete, "/tokens", "token"},
		{http.MethodGet, "/whoami", "token"},
	}

	cases := []struct {
		name       string
		auth       bool
		visibility string
		open       map[string]bool
	}{
		{"no authentication", false, "", map[string]bool{"read": true, "ack": true, "write": true}},
		{"public", true, VisibilityPublic, map[string]bool{"read": true}},
		{"private", true, VisibilityPrivate, map[string]bool{}},
		{"unknown visibility fails closed", true, "bogus", map[string]bool{}},
	}

	denied := []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.Use(gin.Recovery())

			var auth gin.HandlerFunc
			if tc.auth {
				auth = fakeIdentityAuth(Principal{Subject: "user-admin", Permission: PermissionAdmin})
			}
			registerAPIRoutes(r.Group(APIPrefix), auth, tc.visibility)

			want := map[string]bool{}
			for _, rt := range routes {
				// Without authentication nobody could own a token, so those routes are absent.
				if rt.class == "token" && !tc.auth {
					continue
				}
				want[rt.method+" "+APIPrefix+rt.path] = true
			}
			got := map[string]bool{}
			for _, info := range r.Routes() {
				got[info.Method+" "+info.Path] = true
			}
			require.Equal(t, want, got, "every registered route must be listed, with its guard class, in this test")

			for _, rt := range routes {
				if !want[rt.method+" "+APIPrefix+rt.path] {
					continue
				}
				path := APIPrefix + strings.ReplaceAll(rt.path, ":id", "00000000-0000-0000-0000-000000000000")

				anonymous := serve(r, rt.method, path, "")
				if tc.open[rt.class] {
					assert.NotContains(t, denied, anonymous, "%s %s should be open to anonymous callers", rt.method, rt.path)
				} else {
					assert.Equal(t, http.StatusUnauthorized, anonymous, "%s %s should require authentication", rt.method, rt.path)
				}

				if tc.auth {
					signedIn := serve(r, rt.method, path, "session")
					assert.NotContains(t, denied, signedIn, "%s %s should let an admin through", rt.method, rt.path)
				}
			}
		})
	}
}

// TestUnversionedAPIRoutesAreGone checks that the API is only served under APIPrefix.
func TestUnversionedAPIRoutesAreGone(t *testing.T) {
	engine, err := newGinEngine(Options{})
	if err != nil {
		t.Fatal(err)
	}

	for _, route := range engine.Routes() {
		if route.Path == "/api" || route.Path == "/api/ping" || route.Path == "/api/about" ||
			strings.HasPrefix(route.Path, "/swagger/") {
			continue
		}
		assert.True(t, strings.HasPrefix(route.Path, APIPrefix+"/"),
			"%s %s is served outside %s", route.Method, route.Path, APIPrefix)
	}

	assert.Equal(t, http.StatusNotFound, serve(engine, http.MethodGet, "/api/pipeline/reports", ""))
}

func serve(h http.Handler, method, path, bearer string) int {
	req := httptest.NewRequest(method, path, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}
