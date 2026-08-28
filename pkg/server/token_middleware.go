package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/updatecli/udash/pkg/database"
	"github.com/updatecli/udash/pkg/model"
)

// APITokenPrefix marks a bearer token as one Udash issued itself.
//
// The prefix is what lets the middleware tell an Udash token from an identity
// provider one without having to try both, and lets secret scanners recognize one
// if it ever leaks into a public repository.
const APITokenPrefix = "udash_pat_"

// apiTokenBytes is how much entropy a token carries.
const apiTokenBytes = 32

// generateAPIToken returns a new token and the hash to store for it.
func generateAPIToken() (string, []byte, error) {
	buffer := make([]byte, apiTokenBytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", nil, err
	}

	token := APITokenPrefix + base64.RawURLEncoding.EncodeToString(buffer)

	return token, hashAPIToken(token), nil
}

// hashAPIToken returns what gets stored for a token.
//
// A plain sha256 is enough here, unlike for a password: the token is 32 random
// bytes, so there is no dictionary to run against it.
func hashAPIToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// bearerToken returns the credential presented by a request, if any.
func bearerToken(c *gin.Context) string {
	header := c.GetHeader("Authorization")
	if header == "" {
		return ""
	}

	if len(header) < 7 || !strings.EqualFold(header[:7], "bearer ") {
		return ""
	}

	return strings.TrimSpace(header[7:])
}

// udashTokenAuth authenticates requests presenting an Udash API token, and hands
// everything else to the identity provider middleware.
//
// It runs first and independently of the configured mode: Udash issues and
// validates these tokens itself, so they behave the same whichever provider is in
// use, and they keep working when an identity provider token would have expired.
func udashTokenAuth(resolver RoleResolver, next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c)
		if !strings.HasPrefix(token, APITokenPrefix) {
			next(c)
			return
		}

		stored, err := database.GetAPITokenByHash(c.Request.Context(), hashAPIToken(token))
		if err != nil {
			if !errors.Is(err, database.ErrAPITokenNotFound) {
				logrus.Errorf("looking up an API token: %s", err)
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{errMessageType: ErrUnauthenticated})
			return
		}

		if stored.ExpiresAt != nil && stored.ExpiresAt.Before(time.Now()) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{errMessageType: ErrUnauthenticated})
			return
		}

		setPrincipal(c, principalFromToken(c.Request.Context(), resolver, stored))

		// Best effort: the timestamp helps spot an unused or leaked token, it does
		// not authorize anything, so a failure must not fail the request.
		if err := database.TouchAPIToken(c.Request.Context(), stored.ID); err != nil {
			logrus.Debugf("recording the use of token %s: %s", stored.ID, err)
		}

		c.Next()
	}
}

// principalFromToken works out what a token may currently do.
//
// The permission recorded on the token is what its creator could do when it was
// issued. Asking the resolver lets a role revoked at the identity provider take
// effect without having to hunt down the tokens created before it.
func principalFromToken(ctx context.Context, resolver RoleResolver, token *model.APIToken) Principal {
	recorded := ParsePermission(token.Permission)

	permission, err := resolver.Resolve(ctx, token.Subject, recorded)
	if err != nil {
		logrus.Warningf("resolving the permission of %q: %s", token.Subject, err)
		permission = recorded
	}

	id := token.ID

	return Principal{
		Subject:    token.Subject,
		Permission: permission,
		TokenID:    &id,
		TokenName:  token.Name,
		Scopes:     token.Scopes,
	}
}
