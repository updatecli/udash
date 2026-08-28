package server

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	// VisibilityPublic indicates a public API
	VisibilityPublic string = "public"
	// VisibilityPrivate indicates a private API
	VisibilityPrivate string = "private"
	// visibilityDefault indicate Default visibility
	VisibilityDefault = VisibilityPublic
	// ModeZitadel indicates Zitadel authentication, validating tokens by introspection
	ModeZitadel = "zitadel"
	// ModeOIDC indicates generic OpenID Connect authentication, validating JWT
	// access tokens locally against the issuer signing keys
	ModeOIDC = "oidc"
	// ModeNone indicates no authentication
	ModeNone = "none"

	// DefaultRoleCacheTTL is how long a permission resolved from the identity
	// provider is reused before being looked up again.
	DefaultRoleCacheTTL = 60 * time.Second

	// ZitadelRolesClaim is the token claim Zitadel puts the project roles in.
	ZitadelRolesClaim = "urn:zitadel:iam:org:project:roles"

	// ResolverZitadel resolves the permission behind an Udash API token by asking
	// Zitadel for the current grants of the identity which created it.
	ResolverZitadel = "zitadel"
	// ResolverSnapshot trusts the permission recorded when the token was created.
	ResolverSnapshot = "snapshot"
)

// AuthOptions holds every authentication and authorization setting.
type AuthOptions struct {
	// Mode selects how incoming tokens are validated.
	// Accepted values are: "oidc", "zitadel", "none"
	// Default to "none"
	Mode string
	// Zitadel holds Zitadel specific options
	Zitadel ZitadelOptions
	// OIDC holds generic OpenID Connect options
	OIDC OIDCOptions
	// Roles maps identity provider roles onto Udash permissions
	Roles RolesOptions
	// Visibility defines the visibility of the API
	// Accepted values are: "public", "private"
	// Default to "public"
	Visibility string
}

// ZitadelOptions defines Zitadel specific options
// for authentication
type ZitadelOptions struct {
	// Domain is the Zitadel domain
	// example: xxx.region.zitadel.cloud
	Domain string
	// KeyFile is the path to the service account key file
	// example: /path/to/key.json
	KeyFile string
}

// OIDCOptions defines the settings of the generic OpenID Connect mode. It works
// with any provider issuing JWT access tokens, Zitadel included.
type OIDCOptions struct {
	// The issuer of our token.
	Issuer string
	// The audience of our token.
	Audience []string
}

// RolesOptions describes how the roles carried by a token become Udash permissions.
type RolesOptions struct {
	// Claim is the token claim holding the identity provider roles. Providers
	// disagree both on the name and on the shape: Zitadel uses an object keyed by
	// role name, Keycloak and Auth0 use an array of strings. Both are accepted.
	Claim string
	// Mapping lists, per Udash permission, the identity provider roles granting it.
	Mapping map[string][]string
	// Default is the permission granted to an authenticated identity matching no
	// role at all. It deliberately defaults to the least privileged one.
	Default string
	// Resolver decides how the permission behind an Udash API token is resolved,
	// since such a request carries no identity provider token to read roles from.
	Resolver string
	// CacheTTL is how long a resolved permission is reused before being looked up
	// again. Without it a publish heavy pipeline would query the identity provider
	// on every single report.
	CacheTTL time.Duration
}

// Init fills in the defaults and the environment variable fallbacks, and reports
// what it cannot make sense of.
//
// An error here must stop the server: carrying on with an unusable configuration
// leaves the API unauthenticated, which is the opposite of what was asked for.
func (a *AuthOptions) Init() error {

	if a.Mode == "" {
		a.Mode = os.Getenv("UDASH_AUTH_MODE")
	}
	a.Mode = strings.ToLower(a.Mode)

	switch a.Visibility {
	case VisibilityPublic:
		logrus.Debugf("API visibility set to public")
	case VisibilityPrivate:
		logrus.Debugf("API visibility set to private")
	case "":
		logrus.Debugf("No API visibility set, defaulting to %q", VisibilityDefault)
		a.Visibility = VisibilityDefault
	default:
		return fmt.Errorf("unknown API visibility %q, accepted values are: %q, %q",
			a.Visibility, VisibilityPublic, VisibilityPrivate)
	}

	switch a.Mode {
	case ModeZitadel:
		if a.Zitadel.Domain == "" {
			a.Zitadel.Domain = os.Getenv("UDASH_AUTH_ZITADEL_DOMAIN")
		}
		if a.Zitadel.KeyFile == "" {
			a.Zitadel.KeyFile = os.Getenv("UDASH_AUTH_ZITADEL_KEYFILE")
		}
		if a.Zitadel.Domain == "" {
			return fmt.Errorf("authentication mode %q requires a Zitadel domain", ModeZitadel)
		}
		if a.Zitadel.KeyFile == "" {
			return fmt.Errorf("authentication mode %q requires a Zitadel key file", ModeZitadel)
		}
	case ModeOIDC:
		if a.OIDC.Issuer == "" {
			a.OIDC.Issuer = os.Getenv("UDASH_AUTH_OIDC_ISSUER")
		}
		if len(a.OIDC.Audience) == 0 {
			if audience := os.Getenv("UDASH_AUTH_OIDC_AUDIENCE"); audience != "" {
				a.OIDC.Audience = []string{audience}
			}
		}
		if a.OIDC.Issuer == "" {
			return fmt.Errorf("authentication mode %q requires an issuer", ModeOIDC)
		}
	case ModeNone, "":
		a.Mode = ModeNone
		logrus.Warningf("No authentication configured, every API endpoint is open")
	default:
		return fmt.Errorf("unknown authentication mode %q, accepted values are: %q, %q, %q",
			a.Mode, ModeOIDC, ModeZitadel, ModeNone)
	}

	return a.Roles.init(a.Mode)
}

func (r *RolesOptions) init(mode string) error {
	if r.Claim == "" {
		r.Claim = os.Getenv("UDASH_AUTH_ROLES_CLAIM")
	}
	if r.Default == "" {
		r.Default = os.Getenv("UDASH_AUTH_ROLES_DEFAULT")
	}
	if r.Resolver == "" {
		r.Resolver = os.Getenv("UDASH_AUTH_ROLES_RESOLVER")
	}

	if r.Claim == "" && mode == ModeZitadel {
		r.Claim = ZitadelRolesClaim
	}

	if len(r.Mapping) == 0 {
		r.Mapping = map[string][]string{
			string(PermissionAdmin):     {"udash.admin"},
			string(PermissionPublisher): {"udash.publisher"},
			string(PermissionViewer):    {"udash.viewer"},
		}
	}

	for permission := range r.Mapping {
		if !ParsePermission(permission).IsValid() {
			return fmt.Errorf("unknown permission %q in the role mapping, accepted values are: %q, %q, %q",
				permission, PermissionViewer, PermissionPublisher, PermissionAdmin)
		}
	}

	if r.Default == "" {
		r.Default = string(PermissionViewer)
	}
	if !ParsePermission(r.Default).IsValid() {
		return fmt.Errorf("unknown default permission %q, accepted values are: %q, %q, %q",
			r.Default, PermissionViewer, PermissionPublisher, PermissionAdmin)
	}

	if r.Resolver == "" {
		r.Resolver = ResolverSnapshot
		if mode == ModeZitadel {
			r.Resolver = ResolverZitadel
		}
	}
	switch r.Resolver {
	case ResolverZitadel:
		if mode != ModeZitadel {
			return fmt.Errorf("role resolver %q requires the %q authentication mode", ResolverZitadel, ModeZitadel)
		}
	case ResolverSnapshot:
	default:
		return fmt.Errorf("unknown role resolver %q, accepted values are: %q, %q",
			r.Resolver, ResolverZitadel, ResolverSnapshot)
	}

	if r.CacheTTL == 0 {
		r.CacheTTL = DefaultRoleCacheTTL
	}

	if r.Claim == "" && mode != ModeNone {
		logrus.Warningf("No role claim configured, every authenticated identity gets the %q permission", r.Default)
	}

	return nil
}
