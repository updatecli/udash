package server

import (
	"time"

	"github.com/google/uuid"
)

// DatabaseSCMRow represents a specific scm configuration from the database.
type DatabaseSCMRow struct {
	ID         uuid.UUID
	Branch     string
	URL        string
	Created_at time.Time
	Updated_at time.Time
}
