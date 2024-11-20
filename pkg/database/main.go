package database

import (
	"embed"
	"fmt"
	"log"
	"os"

	"context"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"

	"github.com/sirupsen/logrus"

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

type Options struct {
	// URI defines the DB URI
	URI               string
	MigrationDisabled bool
}

func Connect(o Options) error {

	var err error

	if o.URI != "" {
		if URI != "" {
			logrus.Debugf("URI %q defined from setting file override the value from environment variable  %q", o.URI, URIEnvVariableName)
		}
		URI = o.URI
	}

	poolConfig, err := pgxpool.ParseConfig(URI)
	if err != nil {
		log.Fatalln("Unable to parse DATABASE_URL:", err)
	}

	DB, err = pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		log.Fatalln("Unable to create connection pool:", err)
	}

	logrus.Infoln("database connected")

	return nil
}

func RunMigrationUp() error {
	logrus.Debugln("Running Database migration")
	d, err := iofs.New(fs, "migrations")
	if err != nil {
		log.Fatal(err)
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
	if err != nil && err.Error() != migrate.ErrNoChange.Error() {
		return fmt.Errorf("running migration: %w", err)
	}

	return nil
}
