package database

import (
	"fmt"
	"time"

	"github.com/stephenafamo/bob"
	"github.com/stephenafamo/bob/dialect/psql"
	"github.com/stephenafamo/bob/dialect/psql/dialect"
	"github.com/stephenafamo/bob/dialect/psql/sm"
)

// timeRangeLayout is the layout used to parse the startTime and endTime filters.
const timeRangeLayout = "2006-01-02 15:04:05Z07:00"

// dateRangeFilterParams holds parameters for applying a date range filter to a query.
type dateRangeFilterParams struct {
	Query         *bob.BaseQuery[*dialect.SelectQuery]
	DateRangeDays int
	StartTime     string
	EndTime       string
}

// resolveTimeRange returns the time window, in UTC, described by the provided
// startTime and endTime strings. If both are empty and days is greater than zero,
// the window ends now and starts days days ago. If both are empty and days is not
// greater than zero, both returned times are zero, meaning that no time boundary applies.
func resolveTimeRange(days int, startTime, endTime string) (time.Time, time.Time, error) {

	if startTime == "" && endTime == "" {
		if days <= 0 {
			return time.Time{}, time.Time{}, nil
		}

		end := time.Now().UTC()
		return end.Add(-time.Duration(days) * 24 * time.Hour), end, nil
	}

	if startTime == "" || endTime == "" {
		return time.Time{}, time.Time{}, fmt.Errorf("both startTime %q and endTime %q must be provided for time range filtering", startTime, endTime)
	}

	startT, err := time.Parse(timeRangeLayout, startTime)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parsing startTime: %w", err)
	}
	endT, err := time.Parse(timeRangeLayout, endTime)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parsing endTime: %w", err)
	}

	startTimeUTC := startT.UTC()
	endTimeUTC := endT.UTC()

	if startTimeUTC.After(endTimeUTC) {
		startTimeUTC, endTimeUTC = endTimeUTC, startTimeUTC
	}

	return startTimeUTC, endTimeUTC, nil
}

// applyRangeFilter applies a time range filter to the given query based on the provided
// startTime and endTime strings in RFC3339 format. If both are empty and dateRangeDays is greater than zero,
// it filters records updated within the last dateRangeDays days.
func applyRangeFilter(columnName string, r dateRangeFilterParams) error {

	startTimeUTC, endTimeUTC, err := resolveTimeRange(r.DateRangeDays, r.StartTime, r.EndTime)
	if err != nil {
		return err
	}

	if startTimeUTC.IsZero() && endTimeUTC.IsZero() {
		return nil
	}

	// Without an explicit time range, only the lower boundary is applied so that
	// records updated while the query runs are still returned.
	if r.StartTime == "" && r.EndTime == "" {
		r.Query.Apply(
			sm.Where(
				psql.Raw(columnName+" > ?", startTimeUTC),
			),
		)
		return nil
	}

	r.Query.Apply(
		sm.Where(
			psql.Raw(columnName+" >= ? AND "+columnName+" < ?", startTimeUTC, endTimeUTC),
		),
	)

	return nil
}
