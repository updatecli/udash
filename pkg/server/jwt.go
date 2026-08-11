package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	jwtmiddleware "github.com/auth0/go-jwt-middleware/v2"
	"github.com/auth0/go-jwt-middleware/v2/jwks"
	"github.com/auth0/go-jwt-middleware/v2/validator"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

var (
	// We want this struct to be filled in with
	// our custom claims from the token.
	customClaims = func() validator.CustomClaims {
		return &CustomClaims{}
	}

	// jwtOptions holds the JWT options
	authOption = AuthOptions{}
)

// parseIssuerURL turns a configured issuer into a URL, accepting it either as a bare host
// ("example.eu.auth0.com") or as a full URL ("https://example.eu.auth0.com"). https is
// assumed when no scheme is given.
//
// Beyond that the value is used verbatim, in particular its trailing slash. The validator
// compares the token's `iss` claim against this string exactly, and providers disagree on
// the trailing slash — Auth0 issues one, Zitadel and Keycloak do not — so rewriting it
// would reject otherwise valid tokens.
func parseIssuerURL(issuer string) (*url.URL, error) {
	if issuer == "" {
		return nil, errors.New("no issuer configured")
	}

	if !strings.Contains(issuer, "://") {
		issuer = "https://" + issuer
	}

	issuerURL, err := url.Parse(issuer)
	if err != nil {
		return nil, err
	}

	if issuerURL.Host == "" {
		return nil, fmt.Errorf("issuer %q has no host", issuer)
	}

	return issuerURL, nil
}

// checkJWT builds a gin.HandlerFunc middleware that will check the validity of our JWT.
//
// It must be called once, when the routes are set up, and the returned middleware reused
// for every request: the JWKS provider it builds caches the signing keys of the issuer,
// and rebuilding it per request means fetching them again on every single call.
//
// A setup failure is reported rather than logged: carrying on would leave a nil validator
// behind, which panics on the first request it is asked to authenticate.
func checkJWT() (gin.HandlerFunc, error) {

	issuerURL, err := parseIssuerURL(authOption.Oauth.Issuer)
	if err != nil {
		return nil, fmt.Errorf("parsing the issuer url: %w", err)
	}
	provider := jwks.NewCachingProvider(issuerURL, 5*time.Minute)

	// Set up the validator.
	jwtValidator, err := validator.New(
		provider.KeyFunc,
		validator.RS256,
		issuerURL.String(),
		authOption.Oauth.Audience,
		validator.WithCustomClaims(customClaims),
		validator.WithAllowedClockSkew(30*time.Second),
	)

	if err != nil {
		return nil, fmt.Errorf("setting up the validator: %w", err)
	}

	errorHandler := func(w http.ResponseWriter, r *http.Request, err error) {
		logrus.Errorf("Encountered error while validating JWT: %v", err)
	}

	middleware := jwtmiddleware.New(
		jwtValidator.ValidateToken,
		jwtmiddleware.WithErrorHandler(errorHandler),
	)

	return func(ctx *gin.Context) {
		encounteredError := true
		var handler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
			encounteredError = false
			ctx.Request = r
			ctx.Next()
		}

		middleware.CheckJWT(handler).ServeHTTP(ctx.Writer, ctx.Request)

		if encounteredError {
			ctx.AbortWithStatusJSON(
				http.StatusUnauthorized,
				map[string]string{errMessageType: ErrInvalidJWT},
			)
		}
	}, nil
}
