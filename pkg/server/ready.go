package server

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/updatecli/udash/pkg/database"
)

const (
	// readyTimeout bounds how long a readiness check waits for the database.
	readyTimeout = 2 * time.Second
	// readyCacheTTL is how long a readiness result is reused. The endpoint needs no
	// authentication, so this bounds the database load a flood of requests can cause.
	readyCacheTTL = time.Second
)

// readinessCheck pings the database at most once per readyCacheTTL. Concurrent callers
// wait for the ping in progress rather than starting their own.
type readinessCheck struct {
	mu        sync.Mutex
	checkedAt time.Time
	err       error
}

var readiness = &readinessCheck{}

func (r *readinessCheck) check(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.checkedAt.IsZero() && time.Since(r.checkedAt) < readyCacheTTL {
		return r.err
	}

	ctx, cancel := context.WithTimeout(ctx, readyTimeout)
	defer cancel()

	err := database.Ping(ctx)
	switch {
	case err != nil && r.err == nil:
		logrus.Warningf("readiness check failed: %s", err)
	case err == nil && r.err != nil:
		logrus.Infof("readiness check recovered")
	}

	r.checkedAt, r.err = time.Now(), err
	return err
}

// Ready checks that the API can serve requests, which requires the database.
// @Summary Readiness of the API
// @Description Check that the API and its database are ready to serve requests
// @Tags Ping
// @Success 200 {object} DefaultResponseModel
// @Failure 503 {object} DefaultResponseModel
// @Router /api/ready [get]
func Ready(c *gin.Context) {
	// A client hanging up must not cache a failure for everyone else.
	if err := readiness.check(context.WithoutCancel(c.Request.Context())); err != nil {
		c.JSON(http.StatusServiceUnavailable, DefaultResponseModel{
			Err: "database unavailable",
		})
		return
	}

	c.JSON(http.StatusOK, DefaultResponseModel{
		Message: "ready",
	})
}
