package database

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/updatecli/udash/test"
	"github.com/updatecli/updatecli/pkg/core/reports"
	"github.com/updatecli/updatecli/pkg/core/result"
)

// gcSeed lists, per table, the rows seeded for a garbage collection according to the
// fate expected for them, and what the garbage collection must report.
type gcSeed struct {
	deleted map[string][]string
	kept    map[string][]string
	want    GCResult
}

func TestGarbageCollect(t *testing.T) {
	ctx := context.Background()

	postgresContainer, err := test.SetupDatabase(t, ctx)
	require.NoError(t, err)

	dbURL, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	require.NoError(t, Connect(Options{URI: dbURL}))
	require.NoError(t, RunMigrationUp())

	// Every garbage collection below keeps 30 days of reports. The seeded rows are far
	// enough from that cutoff for the time elapsed during the test not to matter.
	const maxHistoryDays = 30
	now := time.Now().UTC()
	old := now.AddDate(0, 0, -40)
	ancient := now.AddDate(0, 0, -400)
	recent := now.AddDate(0, 0, -1)

	configTables := map[string]string{
		configSourceType:    configSourceTableName,
		configConditionType: configConditionTableName,
		configTargetType:    configTargetTableName,
	}

	insertSCM := func(t *testing.T, name string, createdAt time.Time, lastReportAt *time.Time) string {
		t.Helper()
		id, err := InsertSCM(ctx, "https://example.com/"+name+".git", "main")
		require.NoError(t, err)
		_, err = DB.Exec(ctx,
			"UPDATE scms SET created_at = $1, updated_at = $1, last_pipeline_report_at = $2 WHERE id = $3",
			createdAt, lastReportAt, id)
		require.NoError(t, err)
		return id
	}

	insertLabel := func(t *testing.T, name string, createdAt time.Time, lastReportAt *time.Time) string {
		t.Helper()
		id, err := InsertLabel(ctx, "gc", name)
		require.NoError(t, err)
		_, err = DB.Exec(ctx,
			"UPDATE labels SET created_at = $1, updated_at = $1, last_pipeline_report_at = $2 WHERE id = $3",
			createdAt, lastReportAt, id)
		require.NoError(t, err)
		return id
	}

	insertConfig := func(t *testing.T, resourceType string, createdAt time.Time) string {
		t.Helper()
		id, err := InsertConfigResource(ctx, resourceType, "shell", `{"command": "true"}`)
		require.NoError(t, err)
		_, err = DB.Exec(ctx,
			fmt.Sprintf("UPDATE %s SET created_at = $1, updated_at = $1 WHERE id = $2", configTables[resourceType]),
			createdAt, id)
		require.NoError(t, err)
		return id
	}

	// insertReport publishes a report, then backdates it and points it at the given
	// rows, which InsertReport only does for a complete Updatecli report. configs is
	// keyed by resource type.
	insertReport := func(t *testing.T, at time.Time, scms, labels []string, configs map[string][]string) string {
		t.Helper()
		id, err := InsertReport(ctx, reports.Report{
			Name:       "gc",
			Result:     result.SUCCESS,
			ID:         "gc",
			PipelineID: "gc",
		}, Publisher{})
		require.NoError(t, err)
		_, err = DB.Exec(ctx, `UPDATE pipelineReports SET
			created_at = $1,
			updated_at = $1,
			target_db_scm_ids = $2::text[]::uuid[],
			label_ids = $3::text[]::uuid[],
			config_source_ids = hstore($4::text[], $4::text[]),
			config_condition_ids = hstore($5::text[], $5::text[]),
			config_target_ids = hstore($6::text[], $6::text[])
			WHERE id = $7`,
			at, scms, labels,
			configs[configSourceType], configs[configConditionType], configs[configTargetType],
			id)
		require.NoError(t, err)
		return id
	}

	// seed starts from empty tables, and publishes oldReports reports older than the
	// cutoff next to a single recent one.
	seed := func(t *testing.T, oldReports int) gcSeed {
		t.Helper()
		_, err := DB.Exec(ctx,
			"TRUNCATE pipelineReports, scms, labels, config_sources, config_conditions, config_targets")
		require.NoError(t, err)

		s := gcSeed{deleted: map[string][]string{}, kept: map[string][]string{}}

		scmOnlyOld := insertSCM(t, "only-old", ancient, &old)
		// Published for before migration 000009, so it never got a last report date.
		scmLegacyUnused := insertSCM(t, "legacy-unused", ancient, nil)
		scmStillUsed := insertSCM(t, "still-used", ancient, nil)
		scmFresh := insertSCM(t, "fresh", recent, nil)
		s.deleted["scms"] = []string{scmOnlyOld, scmLegacyUnused}
		s.kept["scms"] = []string{scmStillUsed, scmFresh}
		s.want.SCMs = 2

		labelOnlyOld := insertLabel(t, "only-old", ancient, &old)
		labelStillUsed := insertLabel(t, "still-used", ancient, &recent)
		labelFresh := insertLabel(t, "fresh", recent, nil)
		s.deleted["labels"] = []string{labelOnlyOld}
		s.kept["labels"] = []string{labelStillUsed, labelFresh}
		s.want.Labels = 1

		// The configs the old reports and the recent one point at, per resource type.
		oldConfigs := map[string][]string{}
		recentConfigs := map[string][]string{}
		for resourceType, table := range configTables {
			onlyOld := insertConfig(t, resourceType, ancient)
			stillUsed := insertConfig(t, resourceType, ancient)
			fresh := insertConfig(t, resourceType, recent)
			s.deleted[table] = []string{onlyOld}
			s.kept[table] = []string{stillUsed, fresh}
			oldConfigs[resourceType] = []string{onlyOld, stillUsed}
			recentConfigs[resourceType] = []string{stillUsed}
		}
		s.want.ConfigSources, s.want.ConfigConditions, s.want.ConfigTargets = 1, 1, 1

		for range oldReports {
			id := insertReport(t, old,
				[]string{scmOnlyOld, scmStillUsed}, []string{labelOnlyOld, labelStillUsed}, oldConfigs)
			s.deleted["pipelineReports"] = append(s.deleted["pipelineReports"], id)
		}
		s.want.Reports = int64(oldReports)

		s.kept["pipelineReports"] = []string{
			insertReport(t, recent, []string{scmStillUsed}, []string{labelStillUsed}, recentConfigs),
		}

		return s
	}

	countRows := func(t *testing.T, table string, ids []string) int {
		t.Helper()
		count := 0
		require.NoError(t, DB.QueryRow(ctx,
			fmt.Sprintf("SELECT count(*) FROM %s WHERE id = ANY($1::text[]::uuid[])", table), ids,
		).Scan(&count))
		return count
	}

	assertNothingDeleted := func(t *testing.T, s gcSeed) {
		t.Helper()
		for table, ids := range s.deleted {
			assert.Equal(t, len(ids), countRows(t, table, ids), "nothing must be deleted from %s", table)
		}
	}

	t.Run("a dry run counts what a run then deletes", func(t *testing.T) {
		s := seed(t, 3)

		dryRun, err := GarbageCollect(ctx, GCParams{MaxHistoryDays: maxHistoryDays, BatchSize: 5000, DryRun: true})
		require.NoError(t, err)
		assert.Equal(t, s.want, dryRun)
		assertNothingDeleted(t, s)

		// A batch of a single row forces every table to be deleted over several batches.
		run, err := GarbageCollect(ctx, GCParams{MaxHistoryDays: maxHistoryDays, BatchSize: 1})
		require.NoError(t, err)
		assert.Equal(t, s.want, run)

		for table, ids := range s.deleted {
			assert.Zero(t, countRows(t, table, ids), "unused rows must be deleted from %s", table)
		}
		for table, ids := range s.kept {
			assert.Equal(t, len(ids), countRows(t, table, ids), "used or recent rows must be kept in %s", table)
		}

		again, err := GarbageCollect(ctx, GCParams{MaxHistoryDays: maxHistoryDays, BatchSize: 1})
		require.NoError(t, err)
		assert.Equal(t, GCResult{}, again, "a second run must have nothing left to delete")
	})

	t.Run("a last batch exactly full ends with an empty batch", func(t *testing.T) {
		s := seed(t, 2)

		run, err := GarbageCollect(ctx, GCParams{MaxHistoryDays: maxHistoryDays, BatchSize: 2})
		require.NoError(t, err)
		assert.Equal(t, s.want, run)
	})

	t.Run("refuses a retention under one day", func(t *testing.T) {
		s := seed(t, 1)

		for _, params := range []GCParams{
			{MaxHistoryDays: 0, BatchSize: 5000},
			{MaxHistoryDays: -1, BatchSize: 5000},
			{MaxHistoryDays: maxHistoryDays, BatchSize: 0},
		} {
			_, err := GarbageCollect(ctx, params)
			assert.Error(t, err, "%+v must be refused", params)
		}

		assertNothingDeleted(t, s)
	})

	t.Run("only one run deletes at a time", func(t *testing.T) {
		s := seed(t, 1)

		conn, err := DB.Acquire(ctx)
		require.NoError(t, err)
		// Cleanups run last in first out: the lock is gone before the connection goes
		// back to the pool, even when an assertion fails.
		t.Cleanup(conn.Release)
		t.Cleanup(func() {
			_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock_all()")
		})

		_, err = conn.Exec(ctx, "SELECT pg_advisory_lock($1)", gcAdvisoryLockKey)
		require.NoError(t, err)

		_, err = GarbageCollect(ctx, GCParams{MaxHistoryDays: maxHistoryDays, BatchSize: 5000})
		require.ErrorIs(t, err, ErrGCLocked)
		assertNothingDeleted(t, s)

		dryRun, err := GarbageCollect(ctx, GCParams{MaxHistoryDays: maxHistoryDays, BatchSize: 5000, DryRun: true})
		require.NoError(t, err, "a dry run does not need the lock")
		assert.Equal(t, s.want, dryRun)

		_, err = conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", gcAdvisoryLockKey)
		require.NoError(t, err)

		run, err := GarbageCollect(ctx, GCParams{MaxHistoryDays: maxHistoryDays, BatchSize: 5000})
		require.NoError(t, err)
		assert.Equal(t, s.want, run)
	})
}
