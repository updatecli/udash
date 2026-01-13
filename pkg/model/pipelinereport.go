package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/updatecli/updatecli/pkg/core/reports"
)

// PipelineReport represents a specific pipeline report from the database.
type PipelineReport struct {
	// ID is the unique identifier of the record in the database.
	ID uuid.UUID `json:",omitempty"`
	// Result represent the result of the pipeline execution.
	Result string `json:",omitempty"`
	// Pipeline represent the Updatecli pipeline report.
	Pipeline reports.Report `json:",omitempty"`
	// ReportID represent the ID of the pipeline executed by Updatecli.
	// different execution of the same pipeline will have the same ReportID.
	// This value is coming from the pipeline report to improve the search of reports.
	ReportID string `json:",omitempty"`
	// PipelineID represent the unique identifier of the pipeline.
	// Several reports can be associated to the same PipelineID.
	PipelineID string `json:",omitempty"`
	// TargetScmIDs is a list of unique identifiers of the scm configuration associated with the database.
	TargetScmIDs []uuid.UUID `json:",omitempty"`

	// TargetConfigIDs is a list of unique identifiers of the target configuration associated with the database.
	TargetConfigIDs map[uuid.UUID]string `json:",omitempty"`
	// ConditionConfigIDs is a list of unique identifiers of the condition configuration associated with the database.
	ConditionConfigIDs map[uuid.UUID]string `json:",omitempty"`
	// SourceConfigIDs is a list of unique identifiers of the source configuration associated with the database.
	SourceConfigIDs map[uuid.UUID]string `json:",omitempty"`

	// Create_at represent the creation date of the record.
	Created_at time.Time `json:",omitempty"`
	// Updated_at represent the last update date of the record.
	Updated_at time.Time `json:",omitempty"`
}
