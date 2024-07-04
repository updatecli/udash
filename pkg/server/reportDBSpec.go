package server

import (
	"time"

	"github.com/google/uuid"
	"github.com/updatecli/updatecli/pkg/core/reports"
)

// PipelineReportRow represents a specific pipeline report from the database.
type PipelineReportRow struct {
	ID         uuid.UUID
	Pipeline   reports.Report
	Created_at time.Time
	Updated_at time.Time
}
