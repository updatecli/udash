package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zitadel/zitadel-go/v3/pkg/authorization"
	"github.com/zitadel/zitadel-go/v3/pkg/authorization/oauth"
)

type Interceptor[T authorization.Ctx] struct {
	authorizer *authorization.Authorizer[T]
	// roles describes how the claims of a token become an Udash permission.
	roles RolesOptions
}

func NewZitadelGin[T authorization.Ctx](authorizer *authorization.Authorizer[T], roles RolesOptions) *Interceptor[T] {
	return &Interceptor[T]{
		authorizer: authorizer,
		roles:      roles,
	}
}

func (i *Interceptor[T]) RequireAuthorization(options ...authorization.CheckOption) gin.HandlerFunc {
	return func(c *gin.Context) {
		authCtx, err := i.authorizer.CheckAuthorization(c.Request.Context(), c.GetHeader(authorization.HeaderName), options...)
		if err != nil {
			if errors.Is(err, &authorization.UnauthorizedErr{}) {
				c.JSON(http.StatusUnauthorized, gin.H{errMessageType: err.Error()})
				c.Abort()
				return
			}
			c.JSON(http.StatusForbidden, gin.H{errMessageType: err.Error()})
			c.Abort()
			return
		}
		c.Request = c.Request.WithContext(authorization.WithAuthContext(c.Request.Context(), authCtx))
		setPrincipal(c, i.principal(authCtx))
		c.Next()
	}
}

// principal turns the introspected token into the identity the handlers work with.
func (i *Interceptor[T]) principal(authCtx T) Principal {
	principal := Principal{
		Subject:    authCtx.UserID(),
		Permission: ParsePermission(i.roles.Default),
	}

	// The introspection response carries the claims, but only the concrete type
	// exposes them; authorization.Ctx deliberately does not.
	introspection, ok := any(authCtx).(*oauth.IntrospectionContext)
	if !ok || introspection == nil {
		return principal
	}

	principal.Name = introspection.Username
	principal.Permission = permissionFromRoles(
		rolesFromClaims(introspection.Claims, i.roles.Claim),
		i.roles,
	)

	return principal
}

func (i *Interceptor[T]) Context(ctx context.Context) T {
	return authorization.Context[T](ctx)
}
