package cmd

import (
	"os"

	"github.com/fsnotify/fsnotify"
	"github.com/olblak/udash/pkg/engine"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// Server configuration file
	cfgFile string

	// Verbose allows to enable/disable debug logging
	verbose bool
	rootCmd = &cobra.Command{
		Use:   "udash",
		Short: "udash is another Update monitoring platform",
		PostRun: func(cmd *cobra.Command, args []string) {
			logrus.Infoln("See you next time")
		},
	}
)

// Execute executes the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		logrus.Errorf("%s", err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "set config file")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "debug", "", false, "set log level")

	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		if verbose {
			logrus.SetLevel(logrus.DebugLevel)
		}
	}

	rootCmd.AddCommand(
		versionCmd,
		serverCmd,
	)
}

func initConfig() {

	viper.SetConfigName("config") // name of config file (without extension)
	if cfgFile != "" {
		viper.SetConfigName(cfgFile)
	}

	viper.SetConfigType("yaml")         // REQUIRED if the config file does not have the extension in the name
	viper.AddConfigPath("/etc/udash/")  // path to look for the config file in
	viper.AddConfigPath("$HOME/.udash") // call multiple times to add many search paths
	viper.AddConfigPath(".")            // optionally look for config in the working directory
	// Find and read the config file
	if err := viper.ReadInConfig(); err != nil {
		logrus.Errorln(err)
	}

	viper.OnConfigChange(func(e fsnotify.Event) {
		logrus.Infof("Config file changed:", e.Name)
	})
	viper.WatchConfig()

}

func run(command string) error {

	var o engine.Options

	if err := viper.Unmarshal(&o); err != nil {
		return err
	}

	e := engine.Engine{
		Options: o,
	}

	switch command {
	case "start":
		e.Start()
	default:
		logrus.Warnf("Wrong command %q", command)
	}
	return nil
}
