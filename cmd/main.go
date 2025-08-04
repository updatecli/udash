package cmd

import (
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
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
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	cobra.OnInitialize(initConfig)
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
	viper.AddConfigPath(".")            // optionally look for config in the working directory
	viper.AddConfigPath("$HOME/.udash") // call multiple times to add many search paths
	viper.AddConfigPath("/etc/udash/")  // path to look for the config file in
}
