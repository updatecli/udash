package server

import (
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/updatecli/udash/pkg/database"
	"github.com/updatecli/udash/pkg/model"
)

// CreateTokenRequest is the body of a token creation request.
type CreateTokenRequest struct {
	// Name is what the token is for, shown back in the token list.
	Name string `json:"name" binding:"required"`
	// Scopes is what the token may do. It defaults to publishing reports, and may
	// never exceed what the identity creating it is allowed to do.
	Scopes []string `json:"scopes,omitempty"`
	// ExpiresAt is when the token stops working. Leave it out for a token which
	// never expires, which is what an unattended pipeline needs.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// CreateTokenResponse carries the newly created token.
type CreateTokenResponse struct {
	model.APIToken
	// Token is the credential itself. It is returned here once and never again:
	// only its hash is stored.
	Token string `json:"token"`
}

// WhoamiResponse describes the identity behind the credential used.
type WhoamiResponse struct {
	Subject    string   `json:"subject,omitempty"`
	Name       string   `json:"name,omitempty"`
	Permission string   `json:"permission,omitempty"`
	TokenName  string   `json:"tokenName,omitempty"`
	Scopes     []string `json:"scopes,omitempty"`
}

// registerTokenRoutes wires the API token endpoints.
//
// They live on their own group rather than on /api/pipeline: that group is left
// open for reads when the API is public, which must never apply here.
func registerTokenRoutes(r *gin.Engine, auth gin.HandlerFunc) {
	tokens := r.Group("/api/tokens", auth)

	// Creating a token requires signing in with the identity provider. Letting a
	// token mint another one would let a leaked token outlive its own revocation.
	tokens.POST("", rejectTokenAuth(), requirePermission(PermissionPublisher), CreateAPIToken)
	tokens.GET("", ListAPITokens)
	tokens.DELETE("/:id", DeleteAPIToken)
	tokens.DELETE("", requirePermission(PermissionAdmin), DeleteAPITokensBySubject)

	r.GET("/api/whoami", auth, Whoami)
}

// CreateAPIToken issues a new API token.
//
// @Summary Create an API token
// @Description Issue a long lived token to authenticate against the Udash API. The token is returned once and cannot be recovered afterwards.
// @Tags Tokens
// @Accept json
// @Produce json
// @Param request body CreateTokenRequest true "token to create"
// @Success 201 {object} CreateTokenResponse
// @Failure 400 {object} DefaultResponseModel
// @Failure 401 {object} DefaultResponseModel
// @Failure 403 {object} DefaultResponseModel
// @Security BearerAuth
// @Router /api/tokens [post]
func CreateAPIToken(c *gin.Context) {
	principal, ok := principalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, DefaultResponseModel{Err: ErrUnauthenticated})
		return
	}

	request := CreateTokenRequest{}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, DefaultResponseModel{Err: ErrInvalidTokenRequest})
		return
	}

	allowed := principal.Permission.Scopes()

	scopes := request.Scopes
	if len(scopes) == 0 {
		// Publishing reports is what a token is almost always created for.
		scopes = []string{ScopeReportsWrite}
	}

	// A token must never grant more than the identity creating it.
	for _, scope := range scopes {
		if !slices.Contains(allowed, scope) {
			c.JSON(http.StatusForbidden, DefaultResponseModel{Err: ErrInsufficientScope})
			return
		}
	}

	if request.ExpiresAt != nil && request.ExpiresAt.Before(time.Now()) {
		c.JSON(http.StatusBadRequest, DefaultResponseModel{Err: ErrInvalidTokenRequest})
		return
	}

	token, hash, err := generateAPIToken()
	if err != nil {
		logrus.Errorf("generating an API token: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{Err: err.Error()})
		return
	}

	created, err := database.InsertAPIToken(
		c.Request.Context(),
		request.Name,
		principal.Subject,
		string(principal.Permission),
		scopes,
		hash,
		request.ExpiresAt,
	)
	if err != nil {
		logrus.Errorf("storing an API token: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{Err: err.Error()})
		return
	}

	c.JSON(http.StatusCreated, CreateTokenResponse{APIToken: *created, Token: token})
}

// ListAPITokens returns the caller's API tokens.
//
// @Summary List API tokens
// @Description List the caller's API tokens. Administrators may list everybody's with all=true. The tokens themselves are never returned.
// @Tags Tokens
// @Produce json
// @Param all query bool false "list every identity's tokens, administrators only"
// @Success 200 {array} model.APIToken
// @Failure 401 {object} DefaultResponseModel
// @Security BearerAuth
// @Router /api/tokens [get]
func ListAPITokens(c *gin.Context) {
	principal, ok := principalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, DefaultResponseModel{Err: ErrUnauthenticated})
		return
	}

	// Default to the caller's own tokens, so listing everybody's has to be asked
	// for explicitly and is refused to anyone but an administrator.
	subject := principal.Subject
	if c.Query("all") == "true" {
		if !principal.Permission.AtLeast(PermissionAdmin) {
			c.JSON(http.StatusForbidden, DefaultResponseModel{Err: ErrInsufficientPermission})
			return
		}
		subject = ""
	}

	tokens, err := database.ListAPITokens(c.Request.Context(), subject)
	if err != nil {
		logrus.Errorf("listing API tokens: %s", err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{Err: err.Error()})
		return
	}

	c.JSON(http.StatusOK, tokens)
}

// DeleteAPIToken revokes an API token.
//
// @Summary Revoke an API token
// @Description Revoke one of the caller's API tokens. Administrators may revoke anybody's.
// @Tags Tokens
// @Produce json
// @Param id path string true "token id"
// @Success 200 {object} DefaultResponseModel
// @Failure 401 {object} DefaultResponseModel
// @Failure 404 {object} DefaultResponseModel
// @Security BearerAuth
// @Router /api/tokens/{id} [delete]
func DeleteAPIToken(c *gin.Context) {
	principal, ok := principalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, DefaultResponseModel{Err: ErrUnauthenticated})
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, DefaultResponseModel{Err: ErrTokenNotFound})
		return
	}

	// Restricting the delete to the caller's own subject is what stops one identity
	// revoking another's tokens. An administrator is not restricted.
	subject := principal.Subject
	if principal.Permission.AtLeast(PermissionAdmin) {
		subject = ""
	}

	if err := database.DeleteAPIToken(c.Request.Context(), id, subject); err != nil {
		if errors.Is(err, database.ErrAPITokenNotFound) {
			c.JSON(http.StatusNotFound, DefaultResponseModel{Err: ErrTokenNotFound})
			return
		}
		logrus.Errorf("deleting API token %s: %s", id, err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{Err: err.Error()})
		return
	}

	c.JSON(http.StatusOK, DefaultResponseModel{Message: "token successfully revoked"})
}

// DeleteAPITokensBySubject revokes every token of an identity.
//
// @Summary Revoke every token of an identity
// @Description Revoke all API tokens created by a given identity, which is what offboarding somebody needs. Administrators only.
// @Tags Tokens
// @Produce json
// @Param subject query string true "identity provider subject"
// @Success 200 {object} DefaultResponseModel
// @Failure 400 {object} DefaultResponseModel
// @Failure 403 {object} DefaultResponseModel
// @Security BearerAuth
// @Router /api/tokens [delete]
func DeleteAPITokensBySubject(c *gin.Context) {
	subject := c.Query("subject")
	if subject == "" {
		c.JSON(http.StatusBadRequest, DefaultResponseModel{Err: ErrInvalidTokenRequest})
		return
	}

	deleted, err := database.DeleteAPITokensBySubject(c.Request.Context(), subject)
	if err != nil {
		logrus.Errorf("deleting the API tokens of %q: %s", subject, err)
		c.JSON(http.StatusInternalServerError, DefaultResponseModel{Err: err.Error()})
		return
	}

	logrus.Infof("Revoked %d API tokens of %q", deleted, subject)
	c.JSON(http.StatusOK, DefaultResponseModel{Message: "tokens successfully revoked"})
}

// Whoami describes the identity behind the credential used.
//
// @Summary Describe the current identity
// @Description Return the identity, permission and token scopes behind the credential used. Updatecli calls it to validate a token at login time.
// @Tags Tokens
// @Produce json
// @Success 200 {object} WhoamiResponse
// @Failure 401 {object} DefaultResponseModel
// @Security BearerAuth
// @Router /api/whoami [get]
func Whoami(c *gin.Context) {
	principal, ok := principalFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, DefaultResponseModel{Err: ErrUnauthenticated})
		return
	}

	c.JSON(http.StatusOK, WhoamiResponse{
		Subject:    principal.Subject,
		Name:       principal.Name,
		Permission: string(principal.Permission),
		TokenName:  principal.TokenName,
		Scopes:     principal.Scopes,
	})
}
