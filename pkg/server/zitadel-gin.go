package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zitadel/zitadel-go/v3/pkg/authorization"
)

type Interceptor[T authorization.Ctx] struct {
	authorizer *authorization.Authorizer[T]
}

func NewZitadelGin[T authorization.Ctx](authorizer *authorization.Authorizer[T]) *Interceptor[T] {
	return &Interceptor[T]{
		authorizer: authorizer,
	}
}

func (i *Interceptor[T]) RequireAuthorization(options ...authorization.CheckOption) gin.HandlerFunc {
	return func(c *gin.Context) {
		authCtx, err := i.authorizer.CheckAuthorization(c.Request.Context(), c.GetHeader(authorization.HeaderName), options...)
		if err != nil {
			if errors.Is(err, &authorization.UnauthorizedErr{}) {
				c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
				c.Abort()
				return
			}
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			c.Abort()
			return
		}
		c.Request = c.Request.WithContext(authorization.WithAuthContext(c.Request.Context(), authCtx))
		c.Next()
	}
}

func (i *Interceptor[T]) Context(ctx context.Context) T {
	return authorization.Context[T](ctx)
}
