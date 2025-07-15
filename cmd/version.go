package cmd

import (
	"github.com/spf13/cobra"
	"github.com/updatecli/udash/pkg/version"
)

var (
	// Version Contains application version
	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print current application version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			version.Show(cmd)
		},
	}
)
