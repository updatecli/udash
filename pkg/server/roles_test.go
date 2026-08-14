package server

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// defaultRolesOptions is the mapping Init falls back on.
func defaultRolesOptions(claim string) RolesOptions {
	return RolesOptions{
		Claim: claim,
		Mapping: map[string][]string{
			string(PermissionAdmin):     {"udash.admin"},
			string(PermissionPublisher): {"udash.publisher"},
			string(PermissionViewer):    {"udash.viewer"},
		},
		Default:  string(PermissionViewer),
		CacheTTL: time.Minute,
	}
}

func TestRolesFromClaims(t *testing.T) {
	testCases := []struct {
		name     string
		claims   string
		claim    string
		expected []string
	}{
		{
			name:     "zitadel puts the role names in the keys of an object",
			claim:    ZitadelRolesClaim,
			claims:   `{"urn:zitadel:iam:org:project:roles":{"udash.admin":{"orgID":"org.example.com"}}}`,
			expected: []string{"udash.admin"},
		},
		{
			name:     "keycloak nests an array of strings",
			claim:    "realm_access.roles",
			claims:   `{"realm_access":{"roles":["udash.publisher","offline_access"]}}`,
			expected: []string{"udash.publisher", "offline_access"},
		},
		{
			name:     "auth0 uses a namespaced array",
			claim:    "https://udash/roles",
			claims:   `{"https://udash/roles":["udash.viewer"]}`,
			expected: []string{"udash.viewer"},
		},
		{
			name:     "a single string is accepted",
			claim:    "role",
			claims:   `{"role":"udash.admin"}`,
			expected: []string{"udash.admin"},
		},
		{
			name:     "a missing claim yields nothing",
			claim:    "nope",
			claims:   `{"realm_access":{"roles":["udash.admin"]}}`,
			expected: []string{},
		},
		{
			name:     "an unconfigured claim yields nothing",
			claim:    "",
			claims:   `{"urn:zitadel:iam:org:project:roles":{"udash.admin":{}}}`,
			expected: []string{},
		},
		{
			name:     "a dotted path stopping on a non object yields nothing",
			claim:    "realm_access.roles.deeper",
			claims:   `{"realm_access":{"roles":["udash.admin"]}}`,
			expected: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			claims := map[string]interface{}{}
			require.NoError(t, json.Unmarshal([]byte(tc.claims), &claims))

			assert.ElementsMatch(t, tc.expected, rolesFromClaims(claims, tc.claim))
		})
	}
}

func TestPermissionFromRoles(t *testing.T) {
	opts := defaultRolesOptions(ZitadelRolesClaim)

	testCases := []struct {
		name     string
		roles    []string
		expected Permission
	}{
		{"no role falls back on the default", nil, PermissionViewer},
		{"an unrelated role falls back on the default", []string{"other"}, PermissionViewer},
		{"a mapped role is granted", []string{"udash.publisher"}, PermissionPublisher},
		{"the most privileged role wins", []string{"udash.viewer", "udash.admin", "udash.publisher"}, PermissionAdmin},
		{"order does not matter", []string{"udash.admin", "udash.viewer"}, PermissionAdmin},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, permissionFromRoles(tc.roles, opts))
		})
	}
}

func TestPermissionRanking(t *testing.T) {
	assert.True(t, PermissionAdmin.AtLeast(PermissionPublisher))
	assert.True(t, PermissionPublisher.AtLeast(PermissionViewer))
	assert.True(t, PermissionViewer.AtLeast(PermissionViewer))
	assert.False(t, PermissionViewer.AtLeast(PermissionPublisher))
	assert.False(t, PermissionNone.AtLeast(PermissionViewer))

	// A viewer must not be able to hand out a token which publishes.
	assert.NotContains(t, PermissionViewer.Scopes(), ScopeReportsWrite)
	assert.Contains(t, PermissionPublisher.Scopes(), ScopeReportsWrite)
	assert.Empty(t, PermissionNone.Scopes())
}

func TestSnapshotResolver(t *testing.T) {
	got, err := snapshotResolver{}.Resolve(context.Background(), "user-1", PermissionPublisher)
	require.NoError(t, err)
	assert.Equal(t, PermissionPublisher, got)
}

func TestCachingResolver(t *testing.T) {
	opts := defaultRolesOptions(ZitadelRolesClaim)

	t.Run("resolves from the identity provider and caches", func(t *testing.T) {
		calls := 0
		resolver := newCachingResolver(func(context.Context, string) ([]string, error) {
			calls++
			return []string{"udash.viewer"}, nil
		}, opts)

		// The creator has been demoted since the token was made.
		for range 3 {
			got, err := resolver.Resolve(context.Background(), "user-1", PermissionPublisher)
			require.NoError(t, err)
			assert.Equal(t, PermissionViewer, got)
		}
		assert.Equal(t, 1, calls, "the answer must be cached")
	})

	t.Run("looks up again once the entry expired", func(t *testing.T) {
		calls := 0
		resolver := newCachingResolver(func(context.Context, string) ([]string, error) {
			calls++
			return []string{"udash.viewer"}, nil
		}, opts)

		now := time.Now()
		resolver.now = func() time.Time { return now }

		_, err := resolver.Resolve(context.Background(), "user-1", PermissionPublisher)
		require.NoError(t, err)

		now = now.Add(2 * opts.CacheTTL)

		_, err = resolver.Resolve(context.Background(), "user-1", PermissionPublisher)
		require.NoError(t, err)

		assert.Equal(t, 2, calls)
	})

	t.Run("never grants more than the token was created with", func(t *testing.T) {
		resolver := newCachingResolver(func(context.Context, string) ([]string, error) {
			return []string{"udash.admin"}, nil
		}, opts)

		// The creator was promoted after making the token; the token must not
		// silently gain the new privileges.
		got, err := resolver.Resolve(context.Background(), "user-1", PermissionViewer)
		require.NoError(t, err)
		assert.Equal(t, PermissionViewer, got)
	})

	t.Run("falls back on the recorded permission when the provider is down", func(t *testing.T) {
		resolver := newCachingResolver(func(context.Context, string) ([]string, error) {
			return nil, errors.New("zitadel unreachable")
		}, opts)

		// Publishing has to keep working through an outage, and this cannot
		// escalate: the permission was granted once already.
		got, err := resolver.Resolve(context.Background(), "user-1", PermissionPublisher)
		require.NoError(t, err)
		assert.Equal(t, PermissionPublisher, got)
	})
}

func TestNewRoleResolver(t *testing.T) {
	t.Run("snapshot needs no client", func(t *testing.T) {
		resolver, err := newRoleResolver(AuthOptions{Roles: RolesOptions{Resolver: ResolverSnapshot}}, nil)
		require.NoError(t, err)
		assert.IsType(t, snapshotResolver{}, resolver)
	})

	t.Run("zitadel without a client is an error", func(t *testing.T) {
		_, err := newRoleResolver(AuthOptions{Roles: RolesOptions{Resolver: ResolverZitadel}}, nil)
		require.Error(t, err)
	})

	t.Run("an unknown resolver is an error", func(t *testing.T) {
		_, err := newRoleResolver(AuthOptions{Roles: RolesOptions{Resolver: "nope"}}, nil)
		require.Error(t, err)
	})
}
