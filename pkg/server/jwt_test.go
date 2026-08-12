package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseIssuerURL(t *testing.T) {
	tests := []struct {
		name    string
		issuer  string
		want    string
		wantErr bool
	}{
		{
			name:   "bare host gets https",
			issuer: "example.eu.auth0.com",
			want:   "https://example.eu.auth0.com",
		},
		{
			name:   "explicit https is kept",
			issuer: "https://example.eu.auth0.com",
			want:   "https://example.eu.auth0.com",
		},
		{
			// Auth0 publishes an `iss` claim with a trailing slash, and the validator
			// compares it byte for byte, so the slash must survive untouched.
			name:   "trailing slash is preserved",
			issuer: "https://example.eu.auth0.com/",
			want:   "https://example.eu.auth0.com/",
		},
		{
			// Zitadel and Keycloak publish `iss` without a trailing slash; adding one
			// here would reject every token they sign.
			name:   "no trailing slash is added",
			issuer: "https://instance.zitadel.cloud",
			want:   "https://instance.zitadel.cloud",
		},
		{
			name:   "path component is preserved",
			issuer: "https://keycloak.example/realms/udash",
			want:   "https://keycloak.example/realms/udash",
		},
		{
			name:   "bare host with path gets https",
			issuer: "keycloak.example/realms/udash",
			want:   "https://keycloak.example/realms/udash",
		},
		{
			name:   "http scheme is not rewritten",
			issuer: "http://localhost:8080/realms/udash",
			want:   "http://localhost:8080/realms/udash",
		},
		{
			name:    "empty issuer is rejected",
			issuer:  "",
			wantErr: true,
		},
		{
			name:    "scheme without host is rejected",
			issuer:  "https://",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseIssuerURL(tt.issuer)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got.String())
		})
	}
}
