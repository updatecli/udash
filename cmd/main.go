package cmd

import (
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// Verbose allows to enable/disable debug logging
	verbose bool
	// Configuration file, shared by every command reading one
	cfgFile string
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
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().BoolVarP(&verbose, "debug", "", false, "set log level")
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "set config file")

	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		if verbose {
			logrus.SetLevel(logrus.DebugLevel)
		}
	}

	rootCmd.AddCommand(
		versionCmd,
		serverCmd,
		gcCmd,
	)
}

func initConfig() {
	viper.SetConfigType("yaml") // REQUIRED if the config file does not have the extension in the name

	// An explicit file is read from its path, and reported when missing, rather than
	// looked up by name in the paths below.
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
		return
	}

	viper.SetConfigName("config")       // name of config file (without extension)
	viper.AddConfigPath(".")            // optionally look for config in the working directory
	viper.AddConfigPath("$HOME/.udash") // call multiple times to add many search paths
	viper.AddConfigPath("/etc/udash/")  // path to look for the config file in
}
