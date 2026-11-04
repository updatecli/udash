package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

// shutdownTimeout bounds how long in-flight requests may take to finish on shutdown.
// It stays below the 30s Kubernetes gives a pod before killing it.
const shutdownTimeout = 25 * time.Second

// readHeaderTimeout bounds how long a client may take to send its request headers.
const readHeaderTimeout = 10 * time.Second

// listenAddress returns the address to listen on, honoring PORT as gin's Run did.
func listenAddress() string {
	if port := os.Getenv("PORT"); port != "" {
		return ":" + port
	}
	return ":8080"
}

// serve runs srv until ctx is done, then stops accepting connections and waits up to
// timeout for in-flight requests to finish.
func serve(ctx context.Context, srv *http.Server, timeout time.Duration) error {
	errc := make(chan error, 1)
	go func() {
		logrus.Infof("listening on %s", srv.Addr)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	logrus.Infof("shutting down, waiting up to %s for in-flight requests", timeout)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutting down: %w", err)
	}

	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}
