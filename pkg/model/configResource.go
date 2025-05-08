package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/updatecli/updatecli/pkg/core/pipeline/condition"
	"github.com/updatecli/updatecli/pkg/core/pipeline/source"
	"github.com/updatecli/updatecli/pkg/core/pipeline/target"
)

// ConfigSource represents a specific resource configuration from the database.
type ConfigSource struct {
	// ID is the unique identifier of the record in the database.
	ID uuid.UUID `json:",omitempty"`
	// Kind represent the kind of the resource configuration.
	Kind string `json:",omitempty"`

	// Created_at represent the creation date of the record.
	Created_at time.Time `json:",omitempty"`
	// Updated_at represent the last update date of the record.
	Updated_at time.Time `json:",omitempty"`

	Config source.Config `json:",omitempty"`
}

// ConditionConfig represents a specific resource configuration from the database.
type ConfigCondition struct {
	// ID is the unique identifier of the record in the database.
	ID uuid.UUID `json:",omitempty"`
	// Kind represent the kind of the resource configuration.
	Kind string `json:",omitempty"`

	// Created_at represent the creation date of the record.
	Created_at time.Time `json:",omitempty"`
	// Updated_at represent the last update date of the record.
	Updated_at time.Time `json:",omitempty"`

	Config condition.Config `json:",omitempty"`
}

// ConfigTarget represents a specific resource configuration from the database.
type ConfigTarget struct {
	// ID is the unique identifier of the record in the database.
	ID uuid.UUID `json:",omitempty"`
	// Kind represent the kind of the resource configuration.
	Kind string `json:",omitempty"`

	// Created_at represent the creation date of the record.
	Created_at time.Time `json:",omitempty"`
	// Updated_at represent the last update date of the record.
	Updated_at time.Time `json:",omitempty"`

	Config target.Config `json:",omitempty"`
}
