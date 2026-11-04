package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/updatecli/udash/pkg/database"
)

func TestReadyWithoutDatabase(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previous := database.DB
	t.Cleanup(func() { database.DB = previous })

	unreachable, err := pgxpool.New(context.Background(), "postgres://udash@127.0.0.1:1/udash?connect_timeout=1")
	require.NoError(t, err)
	t.Cleanup(unreachable.Close)

	for name, pool := range map[string]*pgxpool.Pool{"not connected": nil, "unreachable": unreachable} {
		t.Run(name, func(t *testing.T) {
			database.DB = pool

			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/ready", nil)
			Ready(c)

			assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
			assert.JSONEq(t, `{"error":"database unavailable"}`, rec.Body.String())
		})
	}
}
