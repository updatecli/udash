package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// TestAPIRouteGuards checks which routes require authentication for each auth setup.
func TestAPIRouteGuards(t *testing.T) {
	gin.SetMode(gin.TestMode)

	type route struct {
		method, path string
		class        string
	}

	routes := []route{
		{http.MethodGet, "/pipeline/reports", "read"},
		{http.MethodGet, "/pipeline/labels", "read"},
		{http.MethodGet, "/pipeline/config/kinds", "read"},
		{http.MethodPost, "/pipeline/reports/search", "read"},
		{http.MethodPost, "/pipeline/scms/search", "read"},
		{http.MethodPost, "/pipeline/actions/search", "read"},
		{http.MethodPut, "/pipeline/actions/ack", "ack"},
		{http.MethodDelete, "/pipeline/actions/ack", "ack"},
		{http.MethodPost, "/pipeline/reports", "write"},
		{http.MethodPut, "/pipeline/reports/x", "write"},
		{http.MethodDelete, "/pipeline/reports/x", "write"},
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

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.Use(gin.Recovery())

			var auth gin.HandlerFunc
			if tc.auth {
				auth = fakeIdentityAuth(Principal{Subject: "user-admin", Permission: PermissionAdmin})
			}
			registerAPIRoutes(r.Group(APIPrefix), auth, tc.visibility)

			for _, rt := range routes {
				anonymous := serve(r, rt.method, APIPrefix+rt.path, "")
				if tc.open[rt.class] {
					assert.NotContains(t, []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound}, anonymous,
						"%s %s should be open to anonymous callers", rt.method, rt.path)
				} else {
					assert.Equal(t, http.StatusUnauthorized, anonymous,
						"%s %s should require authentication", rt.method, rt.path)
				}

				if tc.auth {
					signedIn := serve(r, rt.method, APIPrefix+rt.path, "session")
					assert.NotContains(t, []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound}, signedIn,
						"%s %s should let an admin through", rt.method, rt.path)
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
