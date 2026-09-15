package cmd

import (
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/updatecli/udash/pkg/engine"
)

var (
	gcDryRun         bool
	gcMaxHistoryDays int

	gcCmd = &cobra.Command{
		Use:   "gc",
		Short: "deletes old reports, and the resources no report uses anymore",
		Long: `Deletes the reports older than gc.maxHistoryDays, then the scms, labels and
resource configs no remaining report references.

The server already runs it periodically once gc.maxHistoryDays is set. This command
runs it once, for instance from a scheduled job.`,
		Run: func(cmd *cobra.Command, args []string) {
			cobra.CheckErr(runGC(cmd))
		},
	}
)

func init() {
	gcCmd.Flags().BoolVar(&gcDryRun, "dry-run", false, "count what would be deleted, without deleting anything")
	gcCmd.Flags().IntVar(&gcMaxHistoryDays, "max-history-days", 0, "days of reports to keep, overrides gc.maxHistoryDays")
}

func runGC(cmd *cobra.Command) error {
	// Unlike the server, a single run can be configured from its flags and the
	// environment alone. A configuration file given explicitly must be read though:
	// carrying on without it could collect another database than the intended one.
	if err := viper.ReadInConfig(); err != nil {
		if _, notFound := errors.AsType[viper.ConfigFileNotFoundError](err); cfgFile != "" || !notFound {
			return fmt.Errorf("reading configuration file: %w", err)
		}
		logrus.Debugf("no configuration file found, relying on flags and environment variables")
	}

	var o engine.Options

	if err := viper.Unmarshal(&o); err != nil {
		return err
	}

	if cmd.Flags().Changed("max-history-days") {
		o.GC.MaxHistoryDays = gcMaxHistoryDays
	}

	e := engine.Engine{
		Options: o,
	}

	result, err := e.GarbageCollect(cmd.Context(), gcDryRun)
	if err != nil {
		return err
	}

	if gcDryRun {
		fmt.Printf("Would delete %s\n", result)
		return nil
	}

	fmt.Printf("Deleted %s\n", result)
	return nil
}
