package server

import (
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/updatecli/udash/pkg/database"
)

// Permission is what an identity is allowed to do in Udash.
type Permission string

const (
	// PermissionNone is granted to an unauthenticated request.
	PermissionNone Permission = ""
	// PermissionViewer may read pipeline reports.
	PermissionViewer Permission = "viewer"
	// PermissionPublisher may publish pipeline reports and create API tokens.
	PermissionPublisher Permission = "publisher"
	// PermissionAdmin may do anything, including managing other identities' tokens.
	PermissionAdmin Permission = "admin"
)

const (
	// ScopeReportsRead allows a token to read pipeline reports.
	ScopeReportsRead = "reports:read"
	// ScopeReportsWrite allows a token to publish pipeline reports.
	ScopeReportsWrite = "reports:write"
)

// principalContextKey is where the authenticated identity is stored on the request.
const principalContextKey = "udash.principal"

// ParsePermission turns a configured string into a Permission. An unknown value
// yields PermissionNone, which IsValid rejects.
func ParsePermission(s string) Permission {
	switch Permission(s) {
	case PermissionViewer:
		return PermissionViewer
	case PermissionPublisher:
		return PermissionPublisher
	case PermissionAdmin:
		return PermissionAdmin
	}
	return PermissionNone
}

// IsValid reports whether the permission is one Udash knows about.
func (p Permission) IsValid() bool {
	return p == PermissionViewer || p == PermissionPublisher || p == PermissionAdmin
}

// rank orders permissions so they can be compared. Higher is more privileged.
func (p Permission) rank() int {
	switch p {
	case PermissionAdmin:
		return 3
	case PermissionPublisher:
		return 2
	case PermissionViewer:
		return 1
	}
	return 0
}

// AtLeast reports whether p grants everything other does.
func (p Permission) AtLeast(other Permission) bool {
	return p.rank() >= other.rank()
}

// Scopes returns the token scopes this permission is allowed to hand out. A token
// can never be granted more than the identity which created it, and never gets to
// manage tokens: a token must not be able to mint another one.
func (p Permission) Scopes() []string {
	switch {
	case p.AtLeast(PermissionPublisher):
		return []string{ScopeReportsRead, ScopeReportsWrite}
	case p.AtLeast(PermissionViewer):
		return []string{ScopeReportsRead}
	}
	return nil
}

// Principal is the identity behind a request.
type Principal struct {
	// Subject is the identity provider subject.
	Subject string
	// Name is a human readable name for that identity, when the provider gives one.
	Name string
	// Permission is what that identity may do, after intersecting the identity
	// provider roles with the scopes of the token in use.
	Permission Permission
	// TokenID is set only when the request authenticated with an Udash API token.
	TokenID *uuid.UUID
	// TokenName is the name of that token.
	TokenName string
	// Scopes is what that token may do. It is nil for an identity provider token,
	// which is bounded by its Permission alone.
	Scopes []string
}

// IsToken reports whether the request authenticated with an Udash API token rather
// than with an identity provider token.
func (p Principal) IsToken() bool {
	return p.TokenID != nil
}

// HasScope reports whether the principal may perform the given action.
//
// An identity provider token carries no scopes, so it is bounded by its permission
// only: anything a publisher may do, it may do.
func (p Principal) HasScope(scope string) bool {
	if !p.IsToken() {
		switch scope {
		case ScopeReportsWrite:
			return p.Permission.AtLeast(PermissionPublisher)
		case ScopeReportsRead:
			return p.Permission.AtLeast(PermissionViewer)
		}
		return false
	}

	return slices.Contains(p.Scopes, scope)
}

// setPrincipal records the authenticated identity on the request.
func setPrincipal(c *gin.Context, p Principal) {
	c.Set(principalContextKey, p)
}

// principalFromContext returns the authenticated identity behind the request, if any.
func principalFromContext(c *gin.Context) (Principal, bool) {
	value, ok := c.Get(principalContextKey)
	if !ok {
		return Principal{}, false
	}

	principal, ok := value.(Principal)
	return principal, ok
}

// publisherFromContext describes who is publishing, for attribution.
//
// It yields an empty Publisher when the request is unauthenticated, which is the
// normal case on an instance running without authentication.
func publisherFromContext(c *gin.Context) database.Publisher {
	principal, ok := principalFromContext(c)
	if !ok || principal.Subject == "" {
		return database.Publisher{}
	}

	subject := principal.Subject

	return database.Publisher{
		Subject: &subject,
		TokenID: principal.TokenID,
	}
}

// requirePermission aborts the request unless the caller is at least as privileged
// as the given permission.
func requirePermission(permission Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := principalFromContext(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{errMessageType: ErrUnauthenticated})
			return
		}

		if !principal.Permission.AtLeast(permission) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{errMessageType: ErrInsufficientPermission})
			return
		}

		c.Next()
	}
}

// requireScope aborts the request unless the caller may perform the given action.
func requireScope(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := principalFromContext(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{errMessageType: ErrUnauthenticated})
			return
		}

		if !principal.HasScope(scope) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{errMessageType: ErrInsufficientScope})
			return
		}

		c.Next()
	}
}

// rejectTokenAuth aborts the request when it authenticated with an Udash API token.
// Minting a token must require an identity provider login, otherwise a leaked token
// could be used to issue fresh ones and outlive its own revocation.
func rejectTokenAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := principalFromContext(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{errMessageType: ErrUnauthenticated})
			return
		}

		if principal.IsToken() {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{errMessageType: ErrTokenCannotMintToken})
			return
		}

		c.Next()
	}
}
