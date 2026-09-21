package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/updatecli/udash/pkg/model"
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
		}, Publisher{})
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
				id, err := InsertReport(ctx, tt.report, Publisher{})
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

	t.Run("republishing a report reuses its scm", func(t *testing.T) {
		// The scm of a target used to be looked up by Branch.Target and inserted with
		// Branch.Source, so as soon as the two differed the lookup of the next report
		// missed the row just written and appended a duplicate. Updatecli pushes its
		// changes to a dedicated branch, which is exactly when they differ, so the scms
		// table grew by one row per published report.
		report := reports.Report{
			Name:       "ci: bump Venom version",
			Result:     result.SUCCESS,
			ID:         "scm-reuse",
			PipelineID: "venom",
			Targets: map[string]*result.Target{
				"venom": {
					Scm: result.SCM{
						URL: "https://example.com/scm-reuse.git",
						Branch: struct {
							Source  string
							Working string
							Target  string
						}{Source: "main", Working: "updatecli_main", Target: "updatecli_bump"},
					},
				},
			},
		}

		for range 3 {
			id, err := InsertReport(ctx, report, Publisher{})
			require.NoError(t, err)
			t.Cleanup(func() {
				_, err := DB.Exec(ctx, "DELETE FROM pipelineReports WHERE id = $1", id)
				assert.NoError(t, err)
			})
		}

		scms, _, err := GetSCM(ctx, GetSCMParams{URL: "https://example.com/scm-reuse.git"})
		require.NoError(t, err)
		t.Cleanup(func() {
			_, err := DB.Exec(ctx, "DELETE FROM scms WHERE url = $1", "https://example.com/scm-reuse.git")
			assert.NoError(t, err)
		})

		require.Len(t, scms, 1)
		// The branch stored has to be the one the lookup uses, otherwise the next report
		// misses it again.
		assert.Equal(t, "updatecli_bump", scms[0].Branch)
	})

	t.Run("migration 000015 indexes the reports carrying an open action", func(t *testing.T) {
		// The jsonpath is inlined in openActionSQLExpr so that it matches the predicate of
		// the partial index. Binding it as a parameter would still return the right counts
		// while silently reading every stored payload again.
		indexed := false
		require.NoError(t, DB.QueryRow(ctx, `
			SELECT count(*) = 1
			FROM pg_indexes
			WHERE tablename = 'pipelinereports'
			  AND indexname = 'idx_pipelinereports_open_action'
			  AND indexdef LIKE '%WHERE jsonb_path_exists%'`,
		).Scan(&indexed))

		assert.True(t, indexed)
	})

	deleteReport := func(t *testing.T, id string) {
		t.Helper()
		t.Cleanup(func() {
			_, err := DB.Exec(ctx, "DELETE FROM pipelineReports WHERE id = $1", id)
			assert.NoError(t, err)
		})
	}

	t.Run("republishing a report reuses its condition config", func(t *testing.T) {
		// Conditions used to be looked up among the target configs, so none was ever found
		// and every published report stored one more copy of each of its conditions.
		report := reports.Report{
			Name:       "condition-reuse",
			Result:     result.SUCCESS,
			ID:         "condition-reuse",
			PipelineID: "condition-reuse",
			Conditions: map[string]*result.Condition{
				"exists": {
					Config: map[string]any{
						"Kind": "shell",
						"Spec": map[string]any{"command": "test -f condition-reuse"},
					},
				},
			},
		}

		conditionConfigIDs := map[string]bool{}
		for range 3 {
			id, err := InsertReport(ctx, report, Publisher{})
			require.NoError(t, err)
			deleteReport(t, id)

			stored, err := SearchReport(ctx, id)
			require.NoError(t, err)

			for conditionConfigID := range stored.ConditionConfigIDs {
				conditionConfigIDs[conditionConfigID] = true
			}
		}

		for conditionConfigID := range conditionConfigIDs {
			t.Cleanup(func() {
				_, err := DB.Exec(ctx, "DELETE FROM config_conditions WHERE id = $1", conditionConfigID)
				assert.NoError(t, err)
			})
		}

		assert.Len(t, conditionConfigIDs, 1, "every report must reference the same condition config")
	})

	t.Run("latest reports keep the latest report of every pipeline", func(t *testing.T) {
		search := func(latest bool) ([]string, int) {
			t.Helper()
			data, totalCount, err := SearchLatestReports(SearchLatestReportsParams{
				Ctx:     ctx,
				Latest:  latest,
				Options: ReportSearchOptions{Days: 1},
			})
			require.NoError(t, err)

			ids := make([]string, 0, len(data))
			for _, report := range data {
				ids = append(ids, report.ID)
			}
			assert.Len(t, ids, totalCount, "a page without limit must hold every counted report")

			return ids, totalCount
		}

		insert := func(pipeline string, age time.Duration) string {
			t.Helper()
			id, err := InsertReport(ctx, reports.Report{
				Name: pipeline, Result: result.SUCCESS, ID: pipeline, PipelineID: pipeline,
			}, Publisher{})
			require.NoError(t, err)
			deleteReport(t, id)

			_, err = DB.Exec(ctx,
				"UPDATE pipelineReports SET created_at = $1, updated_at = $1 WHERE id = $2",
				time.Now().UTC().Add(-age), id)
			require.NoError(t, err)

			return id
		}

		_, latestBefore := search(true)
		_, allBefore := search(false)

		olderA := insert("latest-a", 3*time.Hour)
		oldA := insert("latest-a", 2*time.Hour)
		latestA := insert("latest-a", time.Hour)
		latestB := insert("latest-b", time.Hour)

		latestIDs, latestCount := search(true)
		assert.Equal(t, latestBefore+2, latestCount)
		assert.Subset(t, latestIDs, []string{latestA, latestB})
		assert.NotContains(t, latestIDs, olderA)
		assert.NotContains(t, latestIDs, oldA)

		allIDs, allCount := search(false)
		assert.Equal(t, allBefore+4, allCount)
		assert.Subset(t, allIDs, []string{olderA, oldA, latestA, latestB})
	})

	t.Run("latest reports are ordered newest first", func(t *testing.T) {
		insert := func(pipeline string, age time.Duration) string {
			t.Helper()
			id, err := InsertReport(ctx, reports.Report{
				Name: pipeline, Result: result.FAILURE, ID: pipeline, PipelineID: pipeline,
			}, Publisher{})
			require.NoError(t, err)
			deleteReport(t, id)

			_, err = DB.Exec(ctx,
				"UPDATE pipelineReports SET created_at = $1, updated_at = $1 WHERE id = $2",
				time.Now().UTC().Add(-age), id)
			require.NoError(t, err)

			return id
		}

		// Pipeline ids are chosen so that their alphabetical order differs from their
		// date order: sorting by pipeline would put "order-a" first.
		oldest := insert("order-a", 3*time.Hour)
		insert("order-b", 5*time.Hour)
		newest := insert("order-b", 30*time.Minute)
		middle := insert("order-c", 2*time.Hour)

		search := func(limit int) []string {
			t.Helper()
			data, _, err := SearchLatestReports(SearchLatestReportsParams{
				Ctx:     ctx,
				Latest:  true,
				Results: []string{result.FAILURE},
				Options: ReportSearchOptions{Days: 1},
				Limit:   limit,
				Page:    1,
			})
			require.NoError(t, err)

			ids := []string{}
			for _, report := range data {
				ids = append(ids, report.ID)
			}
			return ids
		}

		ours := []string{}
		for _, id := range search(0) {
			if id == oldest || id == middle || id == newest {
				ours = append(ours, id)
			}
		}
		assert.Equal(t, []string{newest, middle, oldest}, ours)

		// Pages follow the same order, so the first page holds the most recent report.
		assert.Equal(t, []string{newest}, search(1))
	})

	t.Run("latest reports are filtered on the latest report of every pipeline", func(t *testing.T) {
		insert := func(pipeline, pipelineResult, actionURL string, age time.Duration) string {
			t.Helper()
			report := reports.Report{Name: pipeline, Result: pipelineResult, ID: pipeline, PipelineID: pipeline}
			if actionURL != "" {
				report.Actions = map[string]*reports.Action{"default": {ID: "default", Link: actionURL}}
			}

			id, err := InsertReport(ctx, report, Publisher{})
			require.NoError(t, err)
			deleteReport(t, id)

			_, err = DB.Exec(ctx,
				"UPDATE pipelineReports SET created_at = $1, updated_at = $1 WHERE id = $2",
				time.Now().UTC().Add(-age), id)
			require.NoError(t, err)

			return id
		}

		search := func(results []string, openAction *bool) []string {
			t.Helper()
			data, totalCount, err := SearchLatestReports(SearchLatestReportsParams{
				Ctx:        ctx,
				Latest:     true,
				Results:    results,
				OpenAction: openAction,
				Options:    ReportSearchOptions{Days: 1},
			})
			require.NoError(t, err)
			assert.Len(t, data, totalCount, "the count must match the filtered reports")

			ids := []string{}
			for _, report := range data {
				ids = append(ids, report.ID)
			}
			return ids
		}

		// Failed, then fixed: its latest report succeeded, so it is no longer failing.
		fixedFailure := insert("filter-fixed", result.FAILURE, "", 2*time.Hour)
		fixedSuccess := insert("filter-fixed", result.SUCCESS, "", time.Hour)
		// Still failing.
		stillFailing := insert("filter-failing", result.FAILURE, "", time.Hour)
		// A pull request that has since been merged: the latest report carries none.
		mergedOpen := insert("filter-merged", result.ATTENTION, "https://example.com/pr/1", 2*time.Hour)
		insert("filter-merged", result.SUCCESS, "", time.Hour)
		// A pull request still waiting.
		waiting := insert("filter-waiting", result.SUCCESS, "https://example.com/pr/2", time.Hour)

		failing := search([]string{result.FAILURE}, nil)
		assert.Contains(t, failing, stillFailing)
		assert.NotContains(t, failing, fixedFailure, "a pipeline fixed since must not be listed as failing")
		assert.NotContains(t, failing, fixedSuccess)

		open := true
		withOpenAction := search(nil, &open)
		assert.Contains(t, withOpenAction, waiting)
		assert.NotContains(t, withOpenAction, mergedOpen, "a pull request merged since must not be listed as waiting")
	})

	t.Run("summarizes several scms sharing a pipeline", func(t *testing.T) {
		newSCM := func(url string) model.SCM {
			t.Helper()
			id, err := InsertSCM(ctx, url, "main")
			require.NoError(t, err)
			t.Cleanup(func() {
				_, err := DB.Exec(ctx, "DELETE FROM scms WHERE id = $1", id)
				assert.NoError(t, err)
			})

			scms, _, err := GetSCM(ctx, GetSCMParams{ID: id})
			require.NoError(t, err)
			require.Len(t, scms, 1)

			return scms[0]
		}

		insert := func(pipeline, pipelineResult, actionURL string, age time.Duration, scms ...model.SCM) {
			t.Helper()
			report := reports.Report{Name: pipeline, Result: pipelineResult, ID: pipeline, PipelineID: pipeline}
			if actionURL != "" {
				report.Actions = map[string]*reports.Action{"default": {ID: "default", Link: actionURL}}
			}

			id, err := InsertReport(ctx, report, Publisher{})
			require.NoError(t, err)
			deleteReport(t, id)

			scmIDs := []uuid.UUID{}
			for _, scm := range scms {
				scmIDs = append(scmIDs, scm.ID)
			}

			_, err = DB.Exec(ctx,
				"UPDATE pipelineReports SET created_at = $1, updated_at = $1, target_db_scm_ids = $2 WHERE id = $3",
				time.Now().UTC().Add(-age), scmIDs, id)
			require.NoError(t, err)
		}

		shared := newSCM("https://example.com/summary-shared.git")
		other := newSCM("https://example.com/summary-other.git")
		idle := newSCM("https://example.com/summary-idle.git")

		// A pipeline reporting to both scms, which failed before recovering: only its latest
		// report counts, for each of them.
		insert("summary-both", result.FAILURE, "", 2*time.Hour, shared, other)
		insert("summary-both", result.SUCCESS, "", time.Hour, shared, other)
		// A pipeline reporting to one of them only, with a pull request still open.
		insert("summary-one", result.ATTENTION, "https://example.com/pull/1", time.Hour, shared)

		dataset, err := GetSCMSummary(GetSCMSummaryParams{
			Ctx:                    ctx,
			MonitoringDurationDays: 1,
			ScmRows:                []model.SCM{shared, other, idle},
		})
		require.NoError(t, err)

		assert.Equal(t, ScmSummaryData{
			ID:                      shared.ID.String(),
			TotalResultByType:       map[string]int{result.SUCCESS: 1, result.ATTENTION: 1},
			TotalResult:             2,
			TotalActionURLs:         1,
			TotalOpenActionByResult: map[string]int{result.ATTENTION: 1},
		}, dataset.Data[shared.URL]["main"])

		assert.Equal(t, ScmSummaryData{
			ID:                      other.ID.String(),
			TotalResultByType:       map[string]int{result.SUCCESS: 1},
			TotalResult:             1,
			TotalOpenActionByResult: map[string]int{},
		}, dataset.Data[other.URL]["main"])

		// An scm without any report within the range is still listed, with empty counts.
		assert.Equal(t, ScmSummaryData{
			ID:                      idle.ID.String(),
			TotalResultByType:       map[string]int{},
			TotalOpenActionByResult: map[string]int{},
		}, dataset.Data[idle.URL]["main"])
	})

	t.Run("an unknown scm filters every report out", func(t *testing.T) {
		// An scm which resolves to no row used to apply no predicate at all, which widened
		// the search to every report in the table instead of narrowing it to none.
		scmID, err := InsertSCM(ctx, "https://example.com/unknown-scm.git", "main")
		require.NoError(t, err)
		t.Cleanup(func() {
			_, err := DB.Exec(ctx, "DELETE FROM scms WHERE id = $1", scmID)
			assert.NoError(t, err)
		})

		scms, _, err := GetSCM(ctx, GetSCMParams{ID: scmID})
		require.NoError(t, err)
		require.Len(t, scms, 1)

		reportID, err := InsertReport(ctx, reports.Report{
			Name: "unknown-scm", Result: result.SUCCESS, ID: "unknown-scm", PipelineID: "unknown-scm",
		}, Publisher{})
		require.NoError(t, err)
		deleteReport(t, reportID)

		// InsertReport relies on the database defaults for its timestamps, so attaching the
		// scm and moving the report inside the search window both happen here.
		_, err = DB.Exec(ctx,
			"UPDATE pipelineReports SET created_at = $1, updated_at = $1, target_db_scm_ids = $2 WHERE id = $3",
			time.Now().UTC().Add(-time.Hour), []uuid.UUID{scms[0].ID}, reportID)
		require.NoError(t, err)

		search := func(scmID string) ([]SearchLatestReportData, int) {
			t.Helper()
			data, totalCount, err := SearchLatestReports(SearchLatestReportsParams{
				Ctx:     ctx,
				ScmID:   scmID,
				Options: ReportSearchOptions{Days: 1},
			})
			require.NoError(t, err)

			return data, totalCount
		}

		// The scm exists, so its own report is still returned: the filter must exclude
		// without over-filtering.
		data, totalCount := search(scmID)
		assert.Equal(t, 1, totalCount)
		require.Len(t, data, 1)
		assert.Equal(t, reportID, data[0].ID)

		// A well formed uuid matching no scm. Zero is the right answer whatever else the
		// table holds, so this stays independent of the other subtests.
		data, totalCount = search("00000000-0000-0000-0000-000000000000")
		assert.Empty(t, data)
		assert.Equal(t, 0, totalCount)
	})
}
