package server

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/updatecli/udash/docs"
)

// TestSwaggerMatchesRoutes checks that the swagger documentation matches the served routes.
func TestSwaggerMatchesRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	assert.Equal(t, strings.TrimPrefix(APIPrefix, "/api/"), docs.SwaggerInfo.Version)

	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	require.NoError(t, json.Unmarshal([]byte(docs.SwaggerInfo.ReadDoc()), &doc))

	documented := []string{}
	for path, methods := range doc.Paths {
		for method := range methods {
			documented = append(documented, strings.ToUpper(method)+" "+path)
		}
	}

	// Token routes are only registered when authentication is configured.
	engine, err := newGinEngine(context.Background(), Options{})
	require.NoError(t, err)
	withAuth := gin.New()
	registerAPIRoutes(withAuth.Group(APIPrefix), fakeIdentityAuth(Principal{}), VisibilityPublic)

	param := regexp.MustCompile(`:([^/]+)`)
	served := map[string]bool{}
	for _, route := range append(engine.Routes(), withAuth.Routes()...) {
		if strings.HasPrefix(route.Path, "/swagger/") {
			continue
		}
		served[route.Method+" "+param.ReplaceAllString(route.Path, "{$1}")] = true
	}

	routes := []string{}
	for route := range served {
		routes = append(routes, route)
	}

	sort.Strings(documented)
	sort.Strings(routes)
	assert.Equal(t, routes, documented, "run `make docs` after changing a route or its @Router annotation")
}
