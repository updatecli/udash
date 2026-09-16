package gc

import (
	"context"
	"errors"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/updatecli/udash/pkg/database"
)

// startupDelay leaves the server time to start serving before the first garbage
// collection competes with it for the database.
const startupDelay = time.Minute

// ErrDisabled is returned when a garbage collection is requested without a retention.
var ErrDisabled = errors.New("garbage collection is disabled, gc.maxHistoryDays must be set")

// Run garbage collects the database every interval, until ctx is done.
//
// It is meant to run in its own goroutine next to the server. A failed run is logged
// and never stops it, the next one simply tries again.
func Run(ctx context.Context, o Options) {
	if !o.Enabled() {
		logrus.Debugf("garbage collector disabled, set gc.maxHistoryDays to enable it")
		return
	}

	logrus.Infof("garbage collector enabled, keeping %d days of reports and running every %s",
		o.MaxHistoryDays, o.Interval)

	// The timer is only reset once a run is over, so a run lasting longer than the
	// interval does not queue another one right behind it, as a ticker would.
	timer := time.NewTimer(startupDelay)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			result, err := RunOnce(ctx, o, false)
			switch {
			case errors.Is(err, database.ErrGCLocked):
				logrus.Debugf("garbage collection skipped: %s", err)
			case err != nil:
				logrus.Errorf("garbage collection failed: %s", err)
			default:
				logrus.Infof("garbage collection deleted %s", result)
			}

			timer.Reset(o.Interval)
		}
	}
}

// RunOnce garbage collects the database a single time.
func RunOnce(ctx context.Context, o Options, dryRun bool) (database.GCResult, error) {
	return database.GarbageCollect(ctx, database.GCParams{
		MaxHistoryDays: o.MaxHistoryDays,
		BatchSize:      o.BatchSize,
		DryRun:         dryRun,
	})
}
