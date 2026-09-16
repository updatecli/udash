package engine

import (
	"context"
	"fmt"

	"github.com/updatecli/udash/pkg/database"
	"github.com/updatecli/udash/pkg/gc"
	"github.com/updatecli/udash/pkg/server"
)

type Options struct {
	Database database.Options
	Server   server.Options
	GC       gc.Options
}

type Engine struct {
	Options Options
}

func (e *Engine) Start() error {
	if err := e.Options.GC.Init(); err != nil {
		return fmt.Errorf("garbage collector options: %w", err)
	}

	if err := database.Connect(e.Options.Database); err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}

	if !e.Options.Database.MigrationDisabled {
		if err := database.RunMigrationUp(); err != nil {
			return fmt.Errorf("running migrations: %w", err)
		}
	}

	go gc.Run(context.Background(), e.Options.GC)

	s := server.Server{
		Options: e.Options.Server,
	}

	return s.Run()
}

// GarbageCollect runs a single garbage collection, outside of the server.
//
// It never migrates the database: the schema belongs to the server, and a dry run
// must not change anything.
func (e *Engine) GarbageCollect(ctx context.Context, dryRun bool) (database.GCResult, error) {
	if err := e.Options.GC.Init(); err != nil {
		return database.GCResult{}, fmt.Errorf("garbage collector options: %w", err)
	}

	// Checked before connecting, so a missing retention is reported as such.
	if !e.Options.GC.Enabled() {
		return database.GCResult{}, gc.ErrDisabled
	}

	if err := database.Connect(e.Options.Database); err != nil {
		return database.GCResult{}, fmt.Errorf("connecting to database: %w", err)
	}

	return gc.RunOnce(ctx, e.Options.GC, dryRun)
}
