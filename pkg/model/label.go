package model

import (
	"time"

	"github.com/google/uuid"
)

// Label represents a specific label name from the database.
type Label struct {
	// ID is a unique identifier for the label, generated as a UUID.
	ID uuid.UUID `json:"id,omitempty"`
	// Key is the label name
	Key string `json:"key,omitempty"`
	// Value is the list of values associated with the label
	Value string `json:"value,omitempty"`
	// CreatedAt is the time the label was created
	CreatedAt *time.Time `json:"created_at,omitempty"`
	// UpdatedAt is the time the label was last updated
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	// LastPipelineReportAt is the time the label was last used in a pipeline report
	LastPipelineReportAt *time.Time `json:"last_pipeline_report_at,omitempty"`
}
