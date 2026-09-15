package gc

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

const (
	// DefaultInterval is the time between two garbage collections run by the server.
	DefaultInterval = 24 * time.Hour
	// MinInterval prevents the server from garbage collecting in a tight loop.
	MinInterval = time.Minute
	// DefaultBatchSize is the number of reports deleted by a single statement.
	DefaultBatchSize = 5000
)

// Options holds the garbage collector settings.
type Options struct {
	// MaxHistoryDays is how many days of reports are kept. Older reports are deleted,
	// along with the scms, labels and resource configs no remaining report uses.
	// Zero, the default, disables the garbage collector.
	MaxHistoryDays int
	// Interval is the time between two garbage collections run by the server.
	Interval time.Duration
	// BatchSize is the maximum number of reports deleted by a single statement.
	BatchSize int
}

// Init fills in the defaults and the environment variable fallbacks, and reports
// what it cannot make sense of.
func (o *Options) Init() error {
	if o.MaxHistoryDays == 0 {
		if value := os.Getenv("UDASH_GC_MAX_HISTORY_DAYS"); value != "" {
			days, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("parsing UDASH_GC_MAX_HISTORY_DAYS: %w", err)
			}
			o.MaxHistoryDays = days
		}
	}

	if o.Interval == 0 {
		if value := os.Getenv("UDASH_GC_INTERVAL"); value != "" {
			interval, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("parsing UDASH_GC_INTERVAL: %w", err)
			}
			o.Interval = interval
		}
	}

	if o.BatchSize == 0 {
		if value := os.Getenv("UDASH_GC_BATCH_SIZE"); value != "" {
			size, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("parsing UDASH_GC_BATCH_SIZE: %w", err)
			}
			o.BatchSize = size
		}
	}

	if o.MaxHistoryDays < 0 {
		return fmt.Errorf("gc maxHistoryDays must not be negative, got %d", o.MaxHistoryDays)
	}

	if o.Interval == 0 {
		o.Interval = DefaultInterval
	}

	if o.Interval < MinInterval {
		return fmt.Errorf("gc interval must be at least %s, got %s", MinInterval, o.Interval)
	}

	if o.BatchSize == 0 {
		o.BatchSize = DefaultBatchSize
	}

	if o.BatchSize < 0 {
		return fmt.Errorf("gc batchSize must not be negative, got %d", o.BatchSize)
	}

	return nil
}

// Enabled reports whether reports are ever deleted.
func (o Options) Enabled() bool {
	return o.MaxHistoryDays > 0
}
