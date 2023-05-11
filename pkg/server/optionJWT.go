package server

import (
	"os"
)

/*
	Code heavily inspired by https://github.com/auth0/go-jwt-middleware/tree/v2.1.0/examples/gin-example
*/

type JWTOptions struct {
	// The issuer of our token.
	Issuer string

	// The audience of our token.
	Audience []string

	// Mode enable auth0 authentication
	Mode string
}

func (j *JWTOptions) Init() {

	if j.Issuer == "" {
		j.Issuer = os.Getenv("UDASH_JWT_ISSUER")
	}

	if len(j.Audience) == 0 {
		j.Audience = []string{os.Getenv("UDASH_JWT_AUDIENCE")}
	}

	jwtOption = *j
}
