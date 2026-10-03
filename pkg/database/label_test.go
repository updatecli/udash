package database

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/updatecli/udash/test"
	"github.com/updatecli/updatecli/pkg/core/reports"
	"github.com/updatecli/updatecli/pkg/core/result"
)

// The id is rejected before any query runs, so no database is needed.
func TestGetLabelRecordsRejectsMalformedID(t *testing.T) {
	_, _, err := GetLabelRecords(context.Background(), "not-a-uuid", "", "", "", "", 0, 1)
	if !errors.Is(err, ErrInvalidParameter) {
		t.Fatalf("expected an error wrapping ErrInvalidParameter, got %v", err)
	}
}

// A label only remembers its latest report, so a time range ending before it must
// still find the label and return the older reports within the range.
func TestSearchLatestReportsLabelFilterWithPastTimeRange(t *testing.T) {
	ctx := context.Background()

	postgresContainer, err := test.SetupDatabase(t, ctx)
	require.NoError(t, err)

	dbURL, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	require.NoError(t, Connect(Options{URI: dbURL}))
	require.NoError(t, RunMigrationUp())

	labels := map[string]string{"env": "prod"}

	// The report is backdated after being published, while the label keeps the
	// timestamp of its publication, as when newer reports were published since.
	reportID, err := InsertReport(ctx, reports.Report{
		Name:       "label",
		Result:     result.SUCCESS,
		ID:         "label",
		PipelineID: "label",
		Labels:     labels,
	}, Publisher{})
	require.NoError(t, err)

	reportAt := time.Now().UTC().AddDate(0, 0, -10)
	_, err = DB.Exec(ctx,
		"UPDATE pipelineReports SET created_at = $1, updated_at = $1 WHERE id = $2",
		reportAt, reportID)
	require.NoError(t, err)

	startTime := reportAt.Add(-time.Hour).Format(timeRangeLayout)
	endTime := reportAt.Add(time.Hour).Format(timeRangeLayout)

	got, total, err := SearchLatestReports(SearchLatestReportsParams{
		Ctx:       ctx,
		StartTime: startTime,
		EndTime:   endTime,
		Labels:    labels,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, got, 1)
	assert.Equal(t, reportID, got[0].ID)

	_, _, err = SearchLatestReports(SearchLatestReportsParams{
		Ctx:       ctx,
		StartTime: startTime,
		EndTime:   endTime,
		Labels:    map[string]string{"env": "missing"},
	})
	assert.ErrorIs(t, err, ErrInvalidParameter)
}
