package engine

import (
	"fmt"

	"github.com/updatecli/udash/pkg/database"
	"github.com/updatecli/udash/pkg/server"
)

type Options struct {
	Database database.Options
	Server   server.Options
}

type Engine struct {
	Options Options
}

func (e *Engine) Start() error {
	if err := database.Connect(e.Options.Database); err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}

	if !e.Options.Database.MigrationDisabled {
		if err := database.RunMigrationUp(); err != nil {
			return fmt.Errorf("running migrations: %w", err)
		}
	}

	s := server.Server{
		Options: e.Options.Server,
	}

	return s.Run()
}
