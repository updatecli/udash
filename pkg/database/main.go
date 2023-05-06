package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"context"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"

	"github.com/sirupsen/logrus"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	URIEnvVariableName string = "UDASH_DB_URI"
)

var (
	URI string = os.Getenv(URIEnvVariableName)
	DB  *pgxpool.Pool
)

type Options struct {
	// URI defines the DB URI
	URI string
}

func Connect(o Options) error {

	var err error

	if o.URI != "" {
		if URI != "" {
			logrus.Debugf("URI %q defined from setting file override the value from environment variable  %q", o.URI, URIEnvVariableName)
		}
		URI = o.URI
	}

	conn, err := pgx.Connect(context.Background(), URI)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(context.Background())

	err = RunMigrationUp(URI)
	if err != nil {
		return err
	}

	logrus.Infoln("database connected")

	poolConfig, err := pgxpool.ParseConfig(URI)
	if err != nil {
		log.Fatalln("Unable to parse DATABASE_URL:", err)
	}

	DB, err = pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		log.Fatalln("Unable to create connection pool:", err)
	}

	return nil
}

func RunMigrationUp(URI string) error {
	logrus.Debugln("Running Database migration")
	db, err := sql.Open("postgres", URI)
	if err != nil {
		return fmt.Errorf("open database connection: %w", err)
	}

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("init database db driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file://./db/migrations",
		"postgres",
		driver,
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
