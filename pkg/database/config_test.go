package database

import (
	"errors"
	"testing"
)

func TestValidateConfigFilter(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		wantErr bool
	}{
		{name: "object", config: `{"image":"nginx"}`},
		{name: "nested", config: `{"spec":{"versionfilter":{"kind":"semver"}}}`},
		{name: "malformed", config: `{"image":`, wantErr: true},
		{name: "not json", config: `image=nginx`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfigFilter(tt.config)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidParameter) {
					t.Fatalf("expected an error wrapping ErrInvalidParameter, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
