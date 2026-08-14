package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthOptionsInit(t *testing.T) {
	t.Run("no mode defaults to none and public", func(t *testing.T) {
		opts := AuthOptions{}
		require.NoError(t, opts.Init())

		assert.Equal(t, ModeNone, opts.Mode)
		assert.Equal(t, VisibilityPublic, opts.Visibility)
		assert.Equal(t, string(PermissionViewer), opts.Roles.Default)
		assert.Equal(t, ResolverSnapshot, opts.Roles.Resolver)
		assert.Equal(t, DefaultRoleCacheTTL, opts.Roles.CacheTTL)
	})

	t.Run("an unknown mode is rejected", func(t *testing.T) {
		// Regression: an unrecognised mode used to be logged and then ignored,
		// registering no middleware at all and leaving every write endpoint open.
		opts := AuthOptions{Mode: "zitadelx"}
		require.ErrorContains(t, opts.Init(), "unknown authentication mode")
	})

	t.Run("an unknown visibility is rejected", func(t *testing.T) {
		opts := AuthOptions{Mode: ModeNone, Visibility: "sometimes"}
		require.ErrorContains(t, opts.Init(), "unknown API visibility")
	})

	t.Run("the mode is case insensitive", func(t *testing.T) {
		opts := AuthOptions{Mode: "OIDC", OIDC: OIDCOptions{Issuer: "https://example.com"}}
		require.NoError(t, opts.Init())
		assert.Equal(t, ModeOIDC, opts.Mode)
	})

	t.Run("oidc requires an issuer", func(t *testing.T) {
		opts := AuthOptions{Mode: ModeOIDC}
		require.ErrorContains(t, opts.Init(), "requires an issuer")
	})

	t.Run("zitadel requires a domain and a key file", func(t *testing.T) {
		require.ErrorContains(t, (&AuthOptions{Mode: ModeZitadel}).Init(), "requires a Zitadel domain")

		opts := AuthOptions{Mode: ModeZitadel, Zitadel: ZitadelOptions{Domain: "example.zitadel.cloud"}}
		require.ErrorContains(t, opts.Init(), "requires a Zitadel key file")
	})

	t.Run("zitadel defaults the claim and the resolver", func(t *testing.T) {
		opts := AuthOptions{
			Mode:    ModeZitadel,
			Zitadel: ZitadelOptions{Domain: "example.zitadel.cloud", KeyFile: "/tmp/key.json"},
		}
		require.NoError(t, opts.Init())

		assert.Equal(t, ZitadelRolesClaim, opts.Roles.Claim)
		assert.Equal(t, ResolverZitadel, opts.Roles.Resolver)
	})

	t.Run("the zitadel resolver needs the zitadel mode", func(t *testing.T) {
		opts := AuthOptions{
			Mode:  ModeOIDC,
			OIDC:  OIDCOptions{Issuer: "https://example.com"},
			Roles: RolesOptions{Resolver: ResolverZitadel},
		}
		require.ErrorContains(t, opts.Init(), "requires the \"zitadel\" authentication mode")
	})

	t.Run("an unknown permission in the mapping is rejected", func(t *testing.T) {
		opts := AuthOptions{
			Mode:  ModeNone,
			Roles: RolesOptions{Mapping: map[string][]string{"superuser": {"udash.superuser"}}},
		}
		require.ErrorContains(t, opts.Init(), "unknown permission")
	})

	t.Run("an unknown default permission is rejected", func(t *testing.T) {
		opts := AuthOptions{Mode: ModeNone, Roles: RolesOptions{Default: "superuser"}}
		require.ErrorContains(t, opts.Init(), "unknown default permission")
	})

	t.Run("environment variables are used as fallbacks", func(t *testing.T) {
		t.Setenv("UDASH_AUTH_MODE", ModeOIDC)
		t.Setenv("UDASH_AUTH_OIDC_ISSUER", "https://example.com")
		t.Setenv("UDASH_AUTH_OIDC_AUDIENCE", "udash")
		t.Setenv("UDASH_AUTH_ROLES_CLAIM", "realm_access.roles")
		t.Setenv("UDASH_AUTH_ROLES_DEFAULT", string(PermissionPublisher))

		opts := AuthOptions{}
		require.NoError(t, opts.Init())

		assert.Equal(t, ModeOIDC, opts.Mode)
		assert.Equal(t, "https://example.com", opts.OIDC.Issuer)
		assert.Equal(t, []string{"udash"}, opts.OIDC.Audience)
		assert.Equal(t, "realm_access.roles", opts.Roles.Claim)
		assert.Equal(t, string(PermissionPublisher), opts.Roles.Default)
	})

	t.Run("explicit values win over the environment", func(t *testing.T) {
		t.Setenv("UDASH_AUTH_MODE", ModeZitadel)
		t.Setenv("UDASH_AUTH_OIDC_ISSUER", "https://from-env.example.com")

		opts := AuthOptions{
			Mode:  ModeOIDC,
			OIDC:  OIDCOptions{Issuer: "https://explicit.example.com"},
			Roles: RolesOptions{CacheTTL: 5 * time.Second},
		}
		require.NoError(t, opts.Init())

		assert.Equal(t, ModeOIDC, opts.Mode)
		assert.Equal(t, "https://explicit.example.com", opts.OIDC.Issuer)
		assert.Equal(t, 5*time.Second, opts.Roles.CacheTTL)
	})
}

func TestNewGinEngineFailsClosed(t *testing.T) {
	// An unusable configuration must stop the server rather than quietly serve an
	// unauthenticated API.
	_, err := newGinEngine(Options{Auth: AuthOptions{Mode: "zitadelx"}})
	require.ErrorContains(t, err, "unknown authentication mode")
}

func TestNewGinEngineWithoutAuth(t *testing.T) {
	// The default deployment has no authentication at all and must keep working.
	engine, err := newGinEngine(Options{})
	require.NoError(t, err)
	require.NotNil(t, engine)

	for _, route := range engine.Routes() {
		assert.NotEqual(t, "/api/tokens", route.Path,
			"the token endpoints must not exist when nobody can be authenticated")
		assert.NotEqual(t, "/api/whoami", route.Path)
	}
}
