package cmd

import (
	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/updatecli/udash/pkg/engine"
)

var (
	// Server configuration file
	cfgFile string

	serverCmd = &cobra.Command{
		Use:   "server",
		Short: "The Udash server",
	}

	serverStartCmd = &cobra.Command{
		Use:   "start",
		Short: "starts an Udash server",
		Run: func(cmd *cobra.Command, args []string) {
			// Find and read the config file
			cobra.CheckErr(viper.ReadInConfig())

			viper.OnConfigChange(func(e fsnotify.Event) {
				logrus.Infof("Config file changed: %q", e.Name)
			})
			viper.WatchConfig()

			cobra.CheckErr(run())
		},
	}
)

func init() {
	serverCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "set config file")
	serverCmd.AddCommand(
		serverStartCmd,
	)
}

func run() error {
	var o engine.Options

	if err := viper.Unmarshal(&o); err != nil {
		return err
	}

	e := engine.Engine{
		Options: o,
	}

	return e.Start()
}
