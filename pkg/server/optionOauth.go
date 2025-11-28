package server

import (
	"os"

	"github.com/sirupsen/logrus"
)

const (
	// VisibilityPublic indicates a public API
	VisibilityPublic string = "public"
	// VisibilityPrivate indicates a private API
	VisibilityPrivate string = "private"
	// visibilityDefault indicate Default visibility
	VisibilityDefault = VisibilityPublic
	// ModeZitadel indicates Zitadel authentication
	ModeZitadel = "zitadel"
	// ModeOauth indicates Oauth authentication
	ModeOauth = "oauth"
	// ModeNone indicates no authentication
	ModeNone = "none"
)

/*
	Code heavily inspired by https://github.com/auth0/go-jwt-middleware/tree/v2.1.0/examples/gin-example
*/

type AuthOptions struct {
	// Mode enable auth0 authentication
	// Accepted values are: "auth0", "zitadel", "none"
	// Default to "none"
	Mode string
	// Zitadel holds Zitadel specific options
	Zitadel ZitadelOptions
	// Oauth holds Oauth specific options
	Oauth OauthOptions
	// Visibility defines the visibility of the API
	// Accepted values are: "public", "private"
	// Default to "public"
	Visibility string
}

// ZitadelOptions defines Zitadel specific options
// for authentication
type ZitadelOptions struct {
	// Domain is the Zitadel domain
	// example: xxx.region.zitadel.cloud
	Domain string
	// KeyFile is the path to the service account key file
	// example: /path/to/key.json
	KeyFile string
	// Role is the required role to access the API
	Role string
}

type OauthOptions struct {
	// The issuer of our token.
	Issuer string
	// The audience of our token.
	Audience []string
}

func (a *AuthOptions) Init() {

	if a.Mode == "" {
		a.Mode = os.Getenv("UDASH_AUTH_MODE")
	}

	switch a.Visibility {
	case VisibilityPublic:
		logrus.Debugf("API visibility set to public")
	case VisibilityPrivate:
		logrus.Debugf("API visibility set to private")
	case "":
		logrus.Debugf("No API visibility set, defaulting to %q", VisibilityDefault)
		a.Visibility = VisibilityDefault
	default:
		logrus.Errorf("Unknown API visibility %q, accepted values are: %q, %q",
			a.Visibility,
			VisibilityPublic,
			VisibilityPrivate,
		)
	}

	switch a.Mode {
	case ModeZitadel:
		if a.Zitadel.Domain == "" {
			a.Zitadel.Domain = os.Getenv("UDASH_AUTH_ZITADEL_DOMAIN")
		}
		if a.Zitadel.KeyFile == "" {
			a.Zitadel.KeyFile = os.Getenv("UDASH_AUTH_ZITADEL_FILEKEY")
		}
	case ModeOauth:
		if a.Oauth.Issuer == "" {
			a.Oauth.Issuer = os.Getenv("UDASH_AUTH_OAUTH_ISSUER")
		}

		if len(a.Oauth.Audience) == 0 {
			a.Oauth.Audience = []string{os.Getenv("UDASH_AUTH_OAUTH_AUDIENCE")}
		}
	case ModeNone, "":
		//
	default:
		logrus.Errorf("Unknown authentication mode %q, accepted values are: %q, %q, %q", a.Mode, ModeOauth, ModeZitadel, ModeNone)
	}

	authOption = *a
}
