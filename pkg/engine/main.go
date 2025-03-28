package engine

import (
	"errors"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/updatecli/udash/pkg/database"
	"github.com/updatecli/udash/pkg/server"
)

var (
	ErrEngineRunFailed error = errors.New("something went wrong in server engine")
)

type Options struct {
	Database database.Options
	Server   server.Options
}

type Engine struct {
	Options Options
}

func (e *Engine) Start() {
	if err := database.Connect(e.Options.Database); err != nil {
		logrus.Errorln(err)
		os.Exit(1)
	}

	if !e.Options.Database.MigrationDisabled {
		if err := database.RunMigrationUp(); err != nil {
			logrus.Errorln(err)
			os.Exit(1)
		}
	}

	s := server.Server{
		Options: e.Options.Server,
	}
	s.Run()
}
