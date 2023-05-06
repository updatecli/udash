package cmd

import (
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	serverCmd = &cobra.Command{
		Use:   "server",
		Short: "The Udash server",
	}

	serverStartCmd = &cobra.Command{
		Use:   "start",
		Short: "starts an Udash server",
		Run: func(cmd *cobra.Command, args []string) {
			err := run("start")
			if err != nil {
				logrus.Errorf("command failed")
				os.Exit(1)
			}
		},
	}
)

func init() {
	serverCmd.AddCommand(
		serverStartCmd,
	)
}
