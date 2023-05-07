package engine

import (
	"errors"
	"os"

	"github.com/olblak/udash/pkg/database"
	"github.com/olblak/udash/pkg/server"
	"github.com/sirupsen/logrus"
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

	if err := database.RunMigrationUp(); err != nil {
		logrus.Errorln(err)
		os.Exit(1)
	}

	s := server.Server{
		Options: e.Options.Server,
	}
	s.Run()
}
