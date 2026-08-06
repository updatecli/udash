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
		// Migration 000004 read "data ->> 'result'" while a marshaled report stores
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

	t.Run("openActionSQLExpr detects an action left open", func(t *testing.T) {
		// This is the contract the whole open action dimension rests on: Updatecli reports
		// a pipeline which had nothing to change as a success even when its change is
		// already waiting in an open pull request, and the only trace of it in the payload
		// is reports.Action.Link, serialized as "actionUrl" and omitted when empty.
		//
		// The expression is exercised through the reports it is meant to tell apart rather
		// than through a handcrafted jsonb document, so that a change to the Action struct
		// of the Updatecli module this repository depends on breaks this test.
		testdata := []struct {
			name   string
			report reports.Report
			want   bool
		}{
			{
				name: "success with a pull request left open",
				report: reports.Report{
					Name:   "succeeded, pull request still open",
					Result: result.SUCCESS,
					ID:     "open-action-success",
					Actions: map[string]*reports.Action{
						"default": {
							ID:   "default",
							Link: "https://github.com/updatecli/udash/pull/42",
						},
					},
				},
				want: true,
			},
			{
				name: "success with an action but no pull request",
				report: reports.Report{
					Name:   "succeeded, nothing to follow up",
					Result: result.SUCCESS,
					ID:     "no-open-action-success",
					Actions: map[string]*reports.Action{
						"default": {ID: "default"},
					},
				},
				want: false,
			},
			{
				name: "pipeline without any action configured",
				report: reports.Report{
					Name:   "no action configured",
					Result: result.SUCCESS,
					ID:     "no-action-at-all",
				},
				want: false,
			},
			{
				name: "attention with a pull request left open",
				report: reports.Report{
					Name:   "changed something and opened a pull request",
					Result: result.ATTENTION,
					ID:     "open-action-attention",
					Actions: map[string]*reports.Action{
						"default": {
							ID:   "default",
							Link: "https://github.com/updatecli/udash/pull/43",
						},
					},
				},
				want: true,
			},
		}

		for _, tt := range testdata {
			t.Run(tt.name, func(t *testing.T) {
				id, err := InsertReport(ctx, tt.report)
				require.NoError(t, err)
				t.Cleanup(func() {
					_, err := DB.Exec(ctx, "DELETE FROM pipelineReports WHERE id = $1", id)
					assert.NoError(t, err)
				})

				got := false
				require.NoError(t, DB.QueryRow(ctx,
					"SELECT "+openActionSQLExpr+" FROM pipelineReports WHERE id = $1", id,
				).Scan(&got))

				assert.Equal(t, tt.want, got)
			})
		}
	})

	t.Run("migration 000011 indexes the open action expression", func(t *testing.T) {
		// The jsonpath is inlined in openActionSQLExpr so that it matches the index
		// expression. Binding it as a parameter would still return the right reports while
		// silently falling back to a sequential scan over every stored payload.
		indexed := false
		require.NoError(t, DB.QueryRow(ctx, `
			SELECT count(*) = 1
			FROM pg_indexes
			WHERE tablename = 'pipelinereports'
			  AND indexname = 'idx_pipelinereports_updated_at_result_open_action'
			  AND indexdef LIKE '%jsonb_path_exists%'`,
		).Scan(&indexed))

		assert.True(t, indexed)
	})
}
