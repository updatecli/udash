package database

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/updatecli/udash/test"
	"github.com/updatecli/updatecli/pkg/core/reports"
	"github.com/updatecli/updatecli/pkg/core/result"
)

func TestDatabase(t *testing.T) {

	ctx := context.Background()

	// This will fail if the database is not setup correctly
	// It does require a local Docker engine to run.
	postgresContainer, err := test.SetupDatabase(t, ctx)
	assert.NoError(t, err, "Failed to setup the database")

	dbURL, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	require.NoError(t, Connect(Options{URI: dbURL}))
	t.Log("Postgres Container connected")
	require.NoError(t, RunMigrationUp())
	t.Log("Postgres Container migrations run")

	t.Run("truncateToBucket matches date_trunc", func(t *testing.T) {
		// The summary zero fills its buckets from truncateToBucket while the counted
		// rows are bucketed by date_trunc. Any divergence between the two silently
		// drops reports from the dataset, so they are compared here rather than left
		// to the endpoint tests to notice.
		granularities := []SummaryGranularity{
			SummaryGranularityHour,
			SummaryGranularityDay,
			SummaryGranularityWeek,
			SummaryGranularityMonth,
		}

		// A monday, a sunday, the first and the last day of a month, a leap day and
		// the boundaries of a day.
		samples := []time.Time{
			time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 1, 5, 13, 45, 12, 0, time.UTC),
			time.Date(2026, 1, 11, 23, 59, 59, 0, time.UTC),
			time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC),
			time.Date(2024, 2, 29, 6, 30, 0, 0, time.UTC),
			time.Date(2026, 12, 31, 23, 0, 0, 0, time.UTC),
		}

		for _, granularity := range granularities {
			for _, sample := range samples {
				want := time.Time{}
				require.NoError(t, DB.QueryRow(ctx,
					"SELECT date_trunc($1, $2::timestamp)", string(granularity), sample,
				).Scan(&want))

				assert.Equal(t, want.UTC(), truncateToBucket(sample, granularity),
					"granularity %q, sample %s", granularity, sample)
			}
		}
	})

	t.Run("migration 000010 backfills pipeline_result", func(t *testing.T) {
		// Migration 000004 read "data ->> 'result'" while a marshalled report stores
		// the key as "Result", so its backfill silently did nothing and every report
		// inserted before it still has an empty pipeline_result.
		id, err := InsertReport(ctx, reports.Report{
			Name:       "ci: bump Venom version",
			Result:     result.SUCCESS,
			ID:         "1de1797bbc925e08e473178425b11eb16fc547291f4b45274da24c2b00e2afc3",
			PipelineID: "venom",
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			_, err := DB.Exec(ctx, "DELETE FROM pipelineReports WHERE id = $1", id)
			assert.NoError(t, err)
		})

		_, err = DB.Exec(ctx,
			"UPDATE pipelineReports SET pipeline_result = '', pipeline_name = '' WHERE id = $1", id)
		require.NoError(t, err)

		// Replaying the migration itself rather than a copy of its statements is what
		// makes this a regression test for the jsonb key casing.
		migration, err := fs.ReadFile("migrations/000010_fix_pipelineReports_denormalized_columns.up.sql")
		require.NoError(t, err)

		_, err = DB.Exec(ctx, string(migration))
		require.NoError(t, err)

		pipelineResult, pipelineName := "", ""
		require.NoError(t, DB.QueryRow(ctx,
			"SELECT pipeline_result, pipeline_name FROM pipelineReports WHERE id = $1", id,
		).Scan(&pipelineResult, &pipelineName))

		assert.Equal(t, result.SUCCESS, pipelineResult)
		assert.Equal(t, "ci: bump Venom version", pipelineName)
	})
}
