package server

import (
	"time"

	"github.com/google/uuid"
)

type PipelineRow struct {
	ID         uuid.UUID
	Pipeline   PipelineReport
	Created_at time.Time
	Updated_at time.Time
}
