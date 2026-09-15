package gc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOptionsInit(t *testing.T) {
	t.Run("disabled with defaults when nothing is set", func(t *testing.T) {
		o := Options{}
		require.NoError(t, o.Init())

		assert.False(t, o.Enabled())
		assert.Equal(t, DefaultInterval, o.Interval)
		assert.Equal(t, DefaultBatchSize, o.BatchSize)
	})

	t.Run("environment variables are fallbacks", func(t *testing.T) {
		t.Setenv("UDASH_GC_MAX_HISTORY_DAYS", "90")
		t.Setenv("UDASH_GC_INTERVAL", "1h")
		t.Setenv("UDASH_GC_BATCH_SIZE", "100")

		o := Options{}
		require.NoError(t, o.Init())

		assert.True(t, o.Enabled())
		assert.Equal(t, 90, o.MaxHistoryDays)
		assert.Equal(t, time.Hour, o.Interval)
		assert.Equal(t, 100, o.BatchSize)
	})

	t.Run("the configuration file wins over the environment", func(t *testing.T) {
		t.Setenv("UDASH_GC_MAX_HISTORY_DAYS", "90")
		t.Setenv("UDASH_GC_INTERVAL", "1h")
		t.Setenv("UDASH_GC_BATCH_SIZE", "100")

		o := Options{MaxHistoryDays: 30, Interval: 2 * time.Hour, BatchSize: 10}
		require.NoError(t, o.Init())

		assert.Equal(t, Options{MaxHistoryDays: 30, Interval: 2 * time.Hour, BatchSize: 10}, o)
	})

	invalid := []struct {
		name string
		o    Options
		env  map[string]string
	}{
		{name: "negative retention", o: Options{MaxHistoryDays: -1}},
		{name: "interval too short", o: Options{Interval: time.Second}},
		{name: "negative batch size", o: Options{BatchSize: -1}},
		{name: "unparsable retention", env: map[string]string{"UDASH_GC_MAX_HISTORY_DAYS": "a month"}},
		{name: "unparsable interval", env: map[string]string{"UDASH_GC_INTERVAL": "daily"}},
		{name: "unparsable batch size", env: map[string]string{"UDASH_GC_BATCH_SIZE": "many"}},
	}

	for _, tt := range invalid {
		t.Run("rejects "+tt.name, func(t *testing.T) {
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			assert.Error(t, tt.o.Init())
		})
	}
}
