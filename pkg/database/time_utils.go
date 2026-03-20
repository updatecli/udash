package database

import (
	"fmt"
	"time"

	"github.com/stephenafamo/bob"
	"github.com/stephenafamo/bob/dialect/psql"
	"github.com/stephenafamo/bob/dialect/psql/dialect"
	"github.com/stephenafamo/bob/dialect/psql/sm"
)

// dateRangeFilterParams holds parameters for applying a date range filter to a query.
type dateRangeFilterParams struct {
	Query         *bob.BaseQuery[*dialect.SelectQuery]
	DateRangeDays int
	StartTime     string
	EndTime       string
}

// applyRangeFilter applies a time range filter to the given query based on the provided
// startTime and endTime strings in RFC3339 format. If both are empty and dateRangeDays is greater than zero,
// it filters records updated within the last dateRangeDays days.
func applyRangeFilter(columnName string, r dateRangeFilterParams) error {

	if r.StartTime == "" && r.EndTime == "" && r.DateRangeDays > 0 {
		start := time.Now().UTC().Add(-time.Duration(r.DateRangeDays) * 24 * time.Hour)
		r.Query.Apply(
			sm.Where(
				psql.Raw(columnName+" > ?", start),
			),
		)
		return nil
	}

	if r.StartTime == "" && r.EndTime == "" {
		return nil
	}

	if r.StartTime == "" || r.EndTime == "" {
		return fmt.Errorf("both startTime %q and endTime %q must be provided for time range filtering", r.StartTime, r.EndTime)
	}

	startT, err := time.Parse("2006-01-02 15:04:05Z07:00", r.StartTime)
	if err != nil {
		return fmt.Errorf("parsing startTime: %w", err)
	}
	endT, err := time.Parse("2006-01-02 15:04:05Z07:00", r.EndTime)
	if err != nil {
		return fmt.Errorf("parsing endTime: %w", err)
	}

	startTimeUTC := startT.UTC()
	endTimeUTC := endT.UTC()

	if startTimeUTC.After(endTimeUTC) {
		startTimeUTC, endTimeUTC = endTimeUTC, startTimeUTC
	}

	r.Query.Apply(
		sm.Where(
			psql.Raw(columnName+" >= ? AND "+columnName+" < ?", startTimeUTC, endTimeUTC),
		),
	)

	return nil
}
