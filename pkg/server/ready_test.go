package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/updatecli/udash/pkg/database"
)

func TestReadyWithoutDatabase(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previous := database.DB
	t.Cleanup(func() {
		database.DB = previous
		readiness = &readinessCheck{}
	})

	unreachable, err := pgxpool.New(context.Background(), "postgres://udash@127.0.0.1:1/udash?connect_timeout=1")
	require.NoError(t, err)
	t.Cleanup(unreachable.Close)

	for name, pool := range map[string]*pgxpool.Pool{"not connected": nil, "unreachable": unreachable} {
		t.Run(name, func(t *testing.T) {
			database.DB = pool
			readiness = &readinessCheck{}

			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/ready", nil)
			Ready(c)

			assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
			assert.JSONEq(t, `{"error":"database unavailable"}`, rec.Body.String())
		})
	}
}

func TestReadyCachesTheResult(t *testing.T) {
	previous := database.DB
	t.Cleanup(func() {
		database.DB = previous
		readiness = &readinessCheck{}
	})

	database.DB = nil
	readiness = &readinessCheck{checkedAt: time.Now()}

	assert.NoError(t, readiness.check(context.Background()), "a fresh result is reused without pinging")

	readiness.checkedAt = time.Now().Add(-2 * readyCacheTTL)
	assert.Error(t, readiness.check(context.Background()), "an expired result pings again")
}
