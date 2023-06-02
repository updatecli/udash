package server

import (
	"os"
)

/*
	Code heavily inspired by https://github.com/auth0/go-jwt-middleware/tree/v2.1.0/examples/gin-example
*/

type AuthOptions struct {
	// The issuer of our token.
	Issuer string

	// The audience of our token.
	Audience []string

	// Mode enable auth0 authentication
	Mode string
}

func (a *AuthOptions) Init() {

	if a.Mode == "" {
		a.Issuer = os.Getenv("UDASH_OAUTH_MODE")
	}

	if a.Issuer == "" {
		a.Issuer = os.Getenv("UDASH_OAUTH_ISSUER")
	}

	if len(a.Audience) == 0 {
		a.Audience = []string{os.Getenv("UDASH_OAUTH_AUDIENCE")}
	}

	authOption = *a
}
