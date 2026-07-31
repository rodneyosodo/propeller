package cli

import (
	"github.com/spf13/cobra"
)

func NewWorkflowsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflows [create]",
		Short: "Workflows manager",
		Long:  `Deploy multi-task workflows (DAGs).`,
	}

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create workflow",
		Long: `Create a workflow with multiple tasks. Tasks with depends_on and run_if
fields are run as a DAG. Tasks are provided as a JSON array either inline with
--tasks or from a file with --tasks-file.

Examples:
  # Inline tasks
  propeller-cli workflows create \
    --tasks '[{"name":"fetch-data","image_url":"docker.io/myorg/fetch:v1"},{"name":"process","image_url":"docker.io/myorg/process:v1","depends_on":["<fetch-data-task-id>"],"run_if":"success"}]'

  # Tasks from a file
  propeller-cli workflows create --tasks-file workflow.json`,
		Run: func(cmd *cobra.Command, args []string) {
			tasks, err := tasksFromFlags(cmd)
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}
			if len(tasks) == 0 {
				logErrorCmd(*cmd, errTasksRequired)

				return
			}

			created, err := psdk.CreateWorkflow(tasks)
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}
			logJSONCmd(*cmd, created)
		},
	}
	createCmd.Flags().String("tasks", "", "JSON array of tasks")
	createCmd.Flags().String("tasks-file", "", "Path to a JSON file containing an array of tasks")

	cmd.AddCommand(createCmd)

	return cmd
}
