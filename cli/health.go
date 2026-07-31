package cli

import (
	"github.com/spf13/cobra"
)

func NewHealthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Check manager health",
		Long:  `Check the health status of the Propeller manager.`,
		Run: func(cmd *cobra.Command, args []string) {
			info, err := psdk.GetHealth()
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}
			logJSONCmd(*cmd, info)
		},
	}
}
