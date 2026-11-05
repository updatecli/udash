package database

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"

	"github.com/sirupsen/logrus"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/golang-migrate/migrate/v4/source/iofs"
)

const (
	URIEnvVariableName string = "UDASH_DB_URI"
)

var (
	URI string = os.Getenv(URIEnvVariableName)
	DB  *pgxpool.Pool
	//go:embed migrations/*.sql
	fs embed.FS
)

// ErrInvalidParameter is wrapped by the errors caused by the parameters of a request
// rather than by the database, so that the API can tell the caller what to fix while
// keeping every other error to itself.
var ErrInvalidParameter = errors.New("invalid parameter")

// defaultConnectTimeout is how long Connect waits for the database by default.
const defaultConnectTimeout = 2 * time.Minute

type Options struct {
	// URI defines the DB URI
	URI               string
	MigrationDisabled bool
	// ConnectTimeout defines how long to wait for the database at startup, 2m by default.
	ConnectTimeout time.Duration
}

// Connect opens the connection pool, and waits for the database to accept connections,
// retrying with backoff until ConnectTimeout or ctx expires.
func Connect(ctx context.Context, o Options) error {
	uri := URI
	if o.URI != "" {
		if URI != "" {
			logrus.Debugf("database URI from the setting file overrides the environment variable %q", URIEnvVariableName)
		}
		uri = o.URI
	}

	poolConfig, err := pgxpool.ParseConfig(uri)
	if err != nil {
		return fmt.Errorf("failed to parse database URI: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return fmt.Errorf("failed to create pgx pool: %w", err)
	}

	timeout := o.ConnectTimeout
	if timeout <= 0 {
		timeout = defaultConnectTimeout
	}

	if err := waitForDatabase(ctx, pool, timeout); err != nil {
		pool.Close()
		return err
	}

	URI = uri
	DB = pool
	logrus.Infoln("database connected")

	return nil
}

// waitForDatabase pings the database until it answers. Errors which retrying cannot fix,
// such as wrong credentials or a missing database, are returned right away.
func waitForDatabase(parent context.Context, pool *pgxpool.Pool, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	backoff := 500 * time.Millisecond
	const maxBackoff = 10 * time.Second

	for {
		err := pool.Ping(ctx)
		if err == nil {
			return nil
		}

		if isPermanent(err) {
			return fmt.Errorf("connecting to database: %w", err)
		}

		logrus.Warningf("database not ready, retrying in %s: %s", backoff, err)

		select {
		case <-ctx.Done():
			// Stopped by the caller, such as a shutdown, rather than by the timeout.
			if parentErr := parent.Err(); parentErr != nil {
				return fmt.Errorf("waiting for database: %w (last error: %v)", parentErr, err)
			}
			return fmt.Errorf("database not ready after %s: %w", timeout, err)
		case <-time.After(backoff):
		}

		backoff = min(backoff*2, maxBackoff)
	}
}

// isPermanent reports whether err comes from PostgreSQL rejecting the connection for a
// reason which retrying cannot fix. When the URI lists several hosts, pgx joins one
// error per host: the error is permanent only when every host failed permanently, as a
// host which is merely down may come back.
func isPermanent(err error) bool {
	switch e := err.(type) {
	case nil:
		return false
	case *pgconn.PgError:
		return isPermanentCode(e.Code)
	case interface{ Unwrap() []error }:
		errs := e.Unwrap()
		if len(errs) == 0 {
			return false
		}
		for _, err := range errs {
			if !isPermanent(err) {
				return false
			}
		}
		return true
	}
	return isPermanent(errors.Unwrap(err))
}

// isPermanentCode reports whether a PostgreSQL error code means retrying cannot help.
func isPermanentCode(code string) bool {
	if len(code) < 2 {
		return false
	}

	switch code[:2] {
	// 28: invalid authorization, 3D: invalid catalog name (the database does not exist).
	case "28", "3D":
		return true
	}
	return false
}

// Ping reports whether the database answers.
func Ping(ctx context.Context) error {
	if DB == nil {
		return errors.New("database not connected")
	}
	return DB.Ping(ctx)
}

// RunMigrationUp applies the pending migrations. It returns as soon as ctx is done, even
// when a migration statement is blocked on a lock, so a signal can stop the startup.
//
// The migration is then abandoned rather than cancelled: the driver ignores contexts, so
// the statement keeps running on the server until it completes, and the migration table
// is left dirty, as it would be if the process were killed.
func RunMigrationUp(ctx context.Context) error {
	done := make(chan error, 1)
	go func() { done <- runMigrationUp() }()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return fmt.Errorf("migration interrupted: %w", ctx.Err())
	}
}

func runMigrationUp() error {
	logrus.Debugln("Running Database migration")
	d, err := iofs.New(fs, "migrations")
	if err != nil {
		return fmt.Errorf("reading migrations: %w", err)
	}

	m, err := migrate.NewWithSourceInstance(
		"iofs",
		d,
		URI,
	)
	if err != nil {
		return fmt.Errorf("loading migration: %w", err)
	}

	err = m.Up()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("running migration: %w", err)
	}

	return nil
}

func stringPtr(s string) *string {
	return &s
}
