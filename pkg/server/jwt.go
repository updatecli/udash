package server

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	jwtmiddleware "github.com/auth0/go-jwt-middleware/v2"
	"github.com/auth0/go-jwt-middleware/v2/validator"
	"github.com/gin-gonic/gin"
)

var (
	// Our token must be signed using this data.
	keyFunc = func(ctx context.Context) (interface{}, error) {
		return jwtOption.SigningKey, nil
	}

	// We want this struct to be filled in with
	// our custom claims from the token.
	customClaims = func() validator.CustomClaims {
		return &CustomClaims{}
	}

	// jwtOptions holds the JWT options
	jwtOption = JWTOptions{}
)

// checkJWT is a gin.HandlerFunc middleware
// that will check the validity of our JWT.
func checkJWT() gin.HandlerFunc {

	var tokenSigningAlg validator.SignatureAlgorithm

	switch strings.ToUpper(jwtOption.tokenSigningAlg) {
	case "RS256":
		tokenSigningAlg = validator.RS256
	case "HS256":
		tokenSigningAlg = validator.HS256
	default:
		log.Fatalf("unsupported signature algorithm: %q", jwtOption.tokenSigningAlg)
	}

	// Set up the validator.
	jwtValidator, err := validator.New(
		keyFunc,
		tokenSigningAlg,
		jwtOption.Issuer,
		jwtOption.Audience,
		validator.WithCustomClaims(customClaims),
		validator.WithAllowedClockSkew(30*time.Second),
	)

	if err != nil {
		log.Fatalf("failed to set up the validator: %v", err)
	}

	errorHandler := func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("Encountered error while validating JWT: %v", err)
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
				map[string]string{"message": "JWT is invalid."},
			)
		}
	}
}
