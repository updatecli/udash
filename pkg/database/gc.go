package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sirupsen/logrus"
)

// gcAdvisoryLockKey identifies the postgres advisory lock held while a garbage
// collection deletes, so that several udash replicas sharing a database do not
// run one at the same time. It spells "udashgc".
const gcAdvisoryLockKey int64 = 0x75646173686763

var (
	// ErrGCLocked is returned when another garbage collection is already running.
	ErrGCLocked = errors.New("another garbage collection is already running")
)

// GCParams defines what a garbage collection deletes.
type GCParams struct {
	// MaxHistoryDays is how many days of reports are kept. It must be at least 1,
	// anything lower would delete every report.
	MaxHistoryDays int
	// BatchSize is the maximum number of rows deleted by a single statement.
	BatchSize int
	// DryRun counts what would be deleted, without deleting anything.
	DryRun bool
}

// GCResult counts, per table, the rows a garbage collection deleted or would delete.
type GCResult struct {
	Reports          int64
	SCMs             int64
	Labels           int64
	ConfigSources    int64
	ConfigConditions int64
	ConfigTargets    int64
}

func (r GCResult) String() string {
	return fmt.Sprintf(
		"%d reports, %d scms, %d labels, %d config sources, %d config conditions, %d config targets",
		r.Reports, r.SCMs, r.Labels, r.ConfigSources, r.ConfigConditions, r.ConfigTargets,
	)
}

// gcTable describes the rows of a table which a garbage collection deletes.
type gcTable struct {
	name string
	// stale matches the rows to delete, on the table aliased as "t", $1 being the cutoff.
	stale string
	// result returns the field of a GCResult counting the rows of this table.
	result func(*GCResult) *int64
}

// unreferenced matches the rows last used before the cutoff which no report
// published since then references.
//
// Nothing in the schema enforces the references from a report to the scms, labels
// and resource configs it uses: they are held in arrays and hstore columns, so
// deleting a report leaves them behind.
//
// Only the reports published since the cutoff are looked at, which is the state once
// the older ones are deleted, so a dry run counts exactly what a run deletes. The age
// condition doubles as a grace period: InsertReport creates those rows before the
// report referencing them, and a row created in between is recent by definition.
//
// It does not cover a row last used before the cutoff which InsertReport looks up
// again at the very moment it is deleted: that report then keeps a dangling id. The
// window is a few milliseconds wide, and a dangling id only hides the resource.
func unreferenced(lastUsed, reference string) string {
	return fmt.Sprintf(`%s < $1 AND NOT EXISTS (
	SELECT 1 FROM pipelineReports r WHERE r.updated_at >= $1 AND %s
)`, lastUsed, reference)
}

// gcLastReportAt dates when an scm or a label was last used.
//
// last_pipeline_report_at is null for the rows no report was published for since
// migrations 000008 and 000009 started tracking it, hence the fallback.
const gcLastReportAt = "COALESCE(t.last_pipeline_report_at, t.updated_at, t.created_at)"

// gcTables lists what a garbage collection deletes, reports first so the lookups of
// the other tables scan fewer of them.
//
// These statements are not built with bob, which takes the hstore ? operator for a
// placeholder. The config hstore columns are keyed by the config id.
var gcTables = []gcTable{
	{
		// Reports are never updated, so updated_at, which is indexed, is their creation date.
		name:   "pipelineReports",
		stale:  "t.updated_at < $1",
		result: func(r *GCResult) *int64 { return &r.Reports },
	},
	{
		name:   "scms",
		stale:  unreferenced(gcLastReportAt, "r.target_db_scm_ids && ARRAY[t.id]"),
		result: func(r *GCResult) *int64 { return &r.SCMs },
	},
	{
		name:   "labels",
		stale:  unreferenced(gcLastReportAt, "r.label_ids && ARRAY[t.id]"),
		result: func(r *GCResult) *int64 { return &r.Labels },
	},
	{
		name:   configSourceTableName,
		stale:  unreferenced("t.updated_at", "r.config_source_ids ? t.id::text"),
		result: func(r *GCResult) *int64 { return &r.ConfigSources },
	},
	{
		name:   configConditionTableName,
		stale:  unreferenced("t.updated_at", "r.config_condition_ids ? t.id::text"),
		result: func(r *GCResult) *int64 { return &r.ConfigConditions },
	},
	{
		name:   configTargetTableName,
		stale:  unreferenced("t.updated_at", "r.config_target_ids ? t.id::text"),
		result: func(r *GCResult) *int64 { return &r.ConfigTargets },
	},
}

// GarbageCollect deletes the reports older than the retention, then the scms, labels
// and resource configs no remaining report references.
//
// Only one garbage collection deletes at a time across every udash instance sharing
// the database, the others return ErrGCLocked. A dry run does not take the lock.
func GarbageCollect(ctx context.Context, params GCParams) (GCResult, error) {
	result := GCResult{}

	if params.MaxHistoryDays < 1 {
		return result, fmt.Errorf("garbage collection must keep at least 1 day of reports, got %d", params.MaxHistoryDays)
	}

	if params.BatchSize < 1 {
		return result, fmt.Errorf("garbage collection batch size must be at least 1, got %d", params.BatchSize)
	}

	// The timestamp columns hold the UTC wall clock, and the driver sends the one of
	// the time location.
	cutoff := time.Now().UTC().AddDate(0, 0, -params.MaxHistoryDays)

	// The advisory lock belongs to a session, so every statement runs on the same
	// connection.
	conn, err := DB.Acquire(ctx)
	if err != nil {
		return result, fmt.Errorf("acquiring a database connection: %w", err)
	}
	defer conn.Release()

	if !params.DryRun {
		unlock, err := lockGC(ctx, conn)
		if err != nil {
			return result, err
		}
		defer unlock()
	}

	for _, table := range gcTables {
		count, err := collect(ctx, conn, table, cutoff, params.BatchSize, params.DryRun)
		*table.result(&result) = count
		if err != nil {
			return result, err
		}
	}

	return result, nil
}

// lockGC takes the garbage collection lock, and returns the function releasing it.
func lockGC(ctx context.Context, conn *pgxpool.Conn) (func(), error) {
	locked := false
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", gcAdvisoryLockKey).Scan(&locked); err != nil {
		return nil, fmt.Errorf("acquiring the garbage collection lock: %w", err)
	}

	if !locked {
		return nil, ErrGCLocked
	}

	return func() {
		// The connection goes back to the pool, still holding the lock if it could not
		// be released. Closing it is what guarantees the lock is gone.
		if _, err := conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", gcAdvisoryLockKey); err != nil {
			logrus.Errorf("releasing the garbage collection lock: %s", err)
			if err := conn.Conn().Close(context.Background()); err != nil {
				logrus.Errorf("closing the garbage collection connection: %s", err)
			}
		}
	}, nil
}

// collect deletes the stale rows of a table in batches, or only counts them on a dry run.
//
// Batches keep every statement short, rather than locking and rewriting years of rows
// at once. Most of these tables have no primary key, so a batch is addressed by ctid,
// which postgres resolves with a tid scan.
func collect(ctx context.Context, conn *pgxpool.Conn, table gcTable, cutoff time.Time, batchSize int, dryRun bool) (int64, error) {
	if dryRun {
		count := int64(0)
		query := fmt.Sprintf("SELECT count(*) FROM %s AS t WHERE %s", table.name, table.stale)
		if err := conn.QueryRow(ctx, query, cutoff).Scan(&count); err != nil {
			return 0, fmt.Errorf("counting stale %s: %w", table.name, err)
		}
		return count, nil
	}

	query := fmt.Sprintf(
		"DELETE FROM %[1]s WHERE ctid = ANY(ARRAY(SELECT t.ctid FROM %[1]s AS t WHERE %[2]s LIMIT $2))",
		table.name, table.stale,
	)

	deleted := int64(0)
	for {
		tag, err := conn.Exec(ctx, query, cutoff, batchSize)
		if err != nil {
			return deleted, fmt.Errorf("deleting stale %s: %w", table.name, err)
		}

		deleted += tag.RowsAffected()
		logrus.Debugf("garbage collection deleted a batch of %d rows from %s", tag.RowsAffected(), table.name)

		if tag.RowsAffected() < int64(batchSize) {
			return deleted, nil
		}
	}
}
