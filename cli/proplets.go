package cli

import (
	"github.com/spf13/cobra"
)

func NewPropletsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "proplets [list|view|delete|sdf|metrics|alive-history]",
		Short: "Proplets manager",
		Long:  `List, view, delete, and inspect proplets.`,
	}

	listCmd := &cobra.Command{
		Use:   cmdListUse,
		Short: "List proplets",
		Long:  `List proplets, optionally filtered by status (active or inactive).`,
		Run: func(cmd *cobra.Command, args []string) {
			status, err := cmd.Flags().GetString("status")
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}

			page, err := psdk.ListProplets(defOffset, defLimit, status)
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}
			logJSONCmd(*cmd, page)
		},
	}
	listCmd.Flags().String("status", "", "Filter by liveness status (active or inactive)")

	viewCmd := &cobra.Command{
		Use:   cmdViewUse,
		Short: "View proplet",
		Long:  `View a single proplet.`,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) != 1 {
				logUsageCmd(*cmd, cmd.Use)

				return
			}

			p, err := psdk.GetProplet(args[0])
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}
			logJSONCmd(*cmd, p)
		},
	}

	deleteCmd := &cobra.Command{
		Use:   cmdDeleteUse,
		Short: "Delete proplet",
		Long:  `Delete a proplet record.`,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) != 1 {
				logUsageCmd(*cmd, cmd.Use)

				return
			}

			if err := psdk.DeleteProplet(args[0]); err != nil {
				logErrorCmd(*cmd, err)

				return
			}
			logOKCmd(*cmd)
		},
	}

	sdfCmd := &cobra.Command{
		Use:   "sdf <id>",
		Short: "View proplet SDF description",
		Long:  `View the Semantic Definition Format (SDF) document describing a proplet.`,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) != 1 {
				logUsageCmd(*cmd, cmd.Use)

				return
			}

			doc, err := psdk.GetPropletSDF(args[0])
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}
			logJSONCmd(*cmd, doc)
		},
	}

	metricsCmd := &cobra.Command{
		Use:   "metrics <id>",
		Short: "View proplet metrics",
		Long:  `View stored metrics for a proplet.`,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) != 1 {
				logUsageCmd(*cmd, cmd.Use)

				return
			}

			page, err := psdk.GetPropletMetrics(args[0], defOffset, defLimit)
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}
			logJSONCmd(*cmd, page)
		},
	}

	aliveHistoryCmd := &cobra.Command{
		Use:   "alive-history <id>",
		Short: "View proplet alive history",
		Long:  `View the paginated heartbeat history for a proplet.`,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) != 1 {
				logUsageCmd(*cmd, cmd.Use)

				return
			}

			page, err := psdk.GetPropletAliveHistory(args[0], defOffset, defLimit)
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}
			logJSONCmd(*cmd, page)
		},
	}

	cmd.AddCommand(listCmd)
	cmd.AddCommand(viewCmd)
	cmd.AddCommand(deleteCmd)
	cmd.AddCommand(sdfCmd)
	cmd.AddCommand(metricsCmd)
	cmd.AddCommand(aliveHistoryCmd)

	cmd.PersistentFlags().Uint64VarP(
		&defOffset,
		"offset",
		"o",
		defOffset,
		"Offset",
	)

	cmd.PersistentFlags().Uint64VarP(
		&defLimit,
		"limit",
		"l",
		defLimit,
		"Limit",
	)

	return cmd
}
