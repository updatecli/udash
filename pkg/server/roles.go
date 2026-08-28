package server

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// rolesFromClaims reads the identity provider roles out of a token's claims.
//
// Providers disagree on the shape of that claim. Zitadel uses an object keyed by
// role name, mapping to the organizations granting it:
//
//	{"urn:zitadel:iam:org:project:roles": {"udash.admin": {"orgID": "org.domain"}}}
//
// Keycloak and Auth0 use an array of strings:
//
//	{"realm_access": {"roles": ["udash.admin"]}}
//
// Both are accepted, and the claim name may be dotted to reach into nested objects.
func rolesFromClaims(claims map[string]interface{}, claim string) []string {
	if claim == "" || len(claims) == 0 {
		return nil
	}

	value, ok := lookupClaim(claims, claim)
	if !ok {
		return nil
	}

	switch typed := value.(type) {
	case map[string]interface{}:
		// Zitadel: the role names are the keys.
		roles := make([]string, 0, len(typed))
		for role := range typed {
			roles = append(roles, role)
		}
		return roles
	case []interface{}:
		roles := make([]string, 0, len(typed))
		for _, entry := range typed {
			if role, ok := entry.(string); ok {
				roles = append(roles, role)
			}
		}
		return roles
	case []string:
		return typed
	case string:
		return []string{typed}
	}

	return nil
}

// lookupClaim finds a claim by name, first verbatim and then by walking a dotted
// path. Zitadel's claim names contain colons but no dots, while Keycloak nests its
// roles under "realm_access.roles", so the verbatim lookup has to come first.
func lookupClaim(claims map[string]interface{}, claim string) (interface{}, bool) {
	if value, ok := claims[claim]; ok {
		return value, true
	}

	parts := strings.Split(claim, ".")
	if len(parts) == 1 {
		return nil, false
	}

	var current interface{} = claims
	for _, part := range parts {
		object, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		current, ok = object[part]
		if !ok {
			return nil, false
		}
	}

	return current, true
}

// permissionFromRoles maps identity provider roles onto the most privileged Udash
// permission they grant, falling back on the configured default.
func permissionFromRoles(roles []string, opts RolesOptions) Permission {
	granted := ParsePermission(opts.Default)

	for permission, names := range opts.Mapping {
		candidate := ParsePermission(permission)
		if !candidate.IsValid() || granted.AtLeast(candidate) {
			continue
		}

		for _, name := range names {
			for _, role := range roles {
				if role == name {
					granted = candidate
					break
				}
			}
		}
	}

	return granted
}

// RoleResolver reports what an identity may currently do, given only its subject.
//
// It exists for requests authenticating with an Udash API token: those carry no
// identity provider token, so there are no claims to read the roles from.
type RoleResolver interface {
	// Resolve returns the current permission of the given subject. The recorded
	// permission is what was granted when the token was created, and is what a
	// resolver returns when it cannot do better.
	Resolve(ctx context.Context, subject string, recorded Permission) (Permission, error)
}

// snapshotResolver trusts the permission recorded when the token was created.
//
// It is the only option for providers without a way to look up a subject's roles.
// Revoking a role at the provider does not downgrade tokens created before, so
// offboarding has to delete the identity's tokens.
type snapshotResolver struct{}

func (snapshotResolver) Resolve(_ context.Context, _ string, recorded Permission) (Permission, error) {
	return recorded, nil
}

// zitadelUserRoles lists the roles currently granted to a subject.
type zitadelUserRoles func(ctx context.Context, subject string) ([]string, error)

// cachingResolver asks the identity provider for the current roles of a subject,
// caching the answer so a publish heavy pipeline does not query it per report.
type cachingResolver struct {
	roles zitadelUserRoles
	opts  RolesOptions

	mu      sync.Mutex
	entries map[string]cacheEntry
	// now is overridable so the cache can be tested without sleeping.
	now func() time.Time
}

type cacheEntry struct {
	permission Permission
	expiresAt  time.Time
}

func newCachingResolver(roles zitadelUserRoles, opts RolesOptions) *cachingResolver {
	return &cachingResolver{
		roles:   roles,
		opts:    opts,
		entries: map[string]cacheEntry{},
		now:     time.Now,
	}
}

func (r *cachingResolver) Resolve(ctx context.Context, subject string, recorded Permission) (Permission, error) {
	if subject == "" {
		return recorded, nil
	}

	r.mu.Lock()
	entry, ok := r.entries[subject]
	r.mu.Unlock()

	if ok && r.now().Before(entry.expiresAt) {
		return entry.permission, nil
	}

	roles, err := r.roles(ctx, subject)
	if err != nil {
		// Falling back on the recorded permission keeps publishing working through a
		// provider outage. It cannot escalate: the recorded permission was already
		// granted once, and is itself bounded by the token's scopes.
		logrus.Warningf("Could not resolve the roles of %q, using the permission recorded on the token: %s", subject, err)
		return recorded, nil
	}

	permission := permissionFromRoles(roles, r.opts)

	// A token never grants more than it was created with, even if its owner has
	// been promoted since.
	if !recorded.AtLeast(permission) {
		permission = recorded
	}

	r.mu.Lock()
	r.entries[subject] = cacheEntry{
		permission: permission,
		expiresAt:  r.now().Add(r.opts.CacheTTL),
	}
	r.mu.Unlock()

	return permission, nil
}

// newRoleResolver builds the resolver named by the configuration.
func newRoleResolver(opts AuthOptions, roles zitadelUserRoles) (RoleResolver, error) {
	switch opts.Roles.Resolver {
	case ResolverSnapshot:
		return snapshotResolver{}, nil
	case ResolverZitadel:
		if roles == nil {
			return nil, fmt.Errorf("role resolver %q needs a Zitadel client", ResolverZitadel)
		}
		return newCachingResolver(roles, opts.Roles), nil
	}

	return nil, fmt.Errorf("unknown role resolver %q", opts.Roles.Resolver)
}
