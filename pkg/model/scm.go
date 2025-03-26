package model

import (
	"time"

	"github.com/google/uuid"
)

// SCM represents a specific scm configuration from the database.
type SCM struct {
	// ID is a unique identifier for the SCM configuration
	ID uuid.UUID `json:",omitempty"`
	// Branch is the Git branch
	Branch string `json:",omitempty"`
	// URL is the Git repository URL
	URL string `json:",omitempty"`
	// Created_at is the time the SCM configuration was created
	Created_at time.Time `json:",omitempty"`
	// Updated_at is the time the SCM configuration was last updated
	Updated_at time.Time `json:",omitempty"`
}
