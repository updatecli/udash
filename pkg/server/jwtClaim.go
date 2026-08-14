package server

import (
	"context"
	"encoding/json"
	"errors"
)

// CustomClaims contains custom data we want from the jwt token.
type CustomClaims struct {
	Name         string `json:"name"`
	Username     string `json:"username"`
	ShouldReject bool   `json:"shouldReject,omitempty"`

	// All keeps every claim of the token, including the ones above.
	//
	// Which claim carries the identity provider roles is configuration, not
	// something that can be named in a struct tag: Zitadel, Keycloak and Auth0
	// each use a different one. See RolesOptions.Claim.
	All map[string]interface{} `json:"-"`
}

// UnmarshalJSON decodes the named claims and keeps the raw ones alongside.
func (c *CustomClaims) UnmarshalJSON(data []byte) error {
	// A local type avoids recursing back into this method.
	type claims CustomClaims

	decoded := claims{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*c = CustomClaims(decoded)

	return json.Unmarshal(data, &c.All)
}

// Validate errors out if `ShouldReject` is true.
func (c *CustomClaims) Validate(ctx context.Context) error {
	if c.ShouldReject {
		return errors.New("should reject was set to true")
	}
	return nil
}
