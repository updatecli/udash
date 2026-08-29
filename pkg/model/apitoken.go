package model

import (
	"time"

	"github.com/google/uuid"
)

// APIToken is a long lived credential Udash issues and validates itself.
//
// It exists because an identity provider access token always expires, while a CI
// pipeline needs a credential it can keep for as long as it runs unattended.
type APIToken struct {
	ID uuid.UUID `json:"id"`
	// Name is what the token is for, chosen by whoever created it.
	Name string `json:"name"`
	// Subject is the identity provider subject which created the token.
	Subject string `json:"subject"`
	// Permission is what that identity could do when the token was issued.
	Permission string `json:"permission"`
	// Scopes is what the token may do, always a subset of what Permission allows.
	Scopes []string `json:"scopes"`
	// CreatedAt is when the token was issued.
	CreatedAt time.Time `json:"created_at"`
	// LastUsedAt is when the token last authenticated a request, if ever.
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	// ExpiresAt is when the token stops working. Nil means it never expires.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}
