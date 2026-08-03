package cli

import (
	"encoding/json"
	"os"

	"github.com/absmach/propeller/pkg/sdk"
	"github.com/spf13/cobra"
)

func NewJobsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "jobs [create|list|view|start|stop]",
		Short: "Jobs manager",
		Long:  `Create, view, list, start, and stop jobs.`,
	}

	createCmd := &cobra.Command{
		Use:   cmdCreateUse,
		Short: "Create job",
		Long: `Create a job with multiple tasks. Tasks are provided as a JSON array
either inline with --tasks or from a file with --tasks-file.

Examples:
  # Inline tasks
  propeller-cli jobs create --name batch --execution-mode sequential \
    --tasks '[{"name":"step-1","image_url":"docker.io/myorg/step1:v1"},{"name":"step-2","image_url":"docker.io/myorg/step2:v1"}]'

  # Tasks from a file
  propeller-cli jobs create --name batch --execution-mode parallel --tasks-file tasks.json`,
		Run: func(cmd *cobra.Command, args []string) {
			name, err := cmd.Flags().GetString("name")
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}
			if name == "" {
				logErrorCmd(*cmd, errNameRequired)

				return
			}

			executionMode, err := cmd.Flags().GetString("execution-mode")
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}

			tasks, err := tasksFromFlags(cmd)
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}
			if len(tasks) == 0 {
				logErrorCmd(*cmd, errTasksRequired)

				return
			}

			job, err := psdk.CreateJob(sdk.JobRequest{
				Name:          name,
				Tasks:         tasks,
				ExecutionMode: executionMode,
			})
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}
			logJSONCmd(*cmd, job)
		},
	}
	createCmd.Flags().String("name", "", "Job name")
	createCmd.Flags().String("execution-mode", "", "Execution mode (parallel, sequential, or configurable)")
	createCmd.Flags().String("tasks", "", "JSON array of tasks")
	createCmd.Flags().String("tasks-file", "", "Path to a JSON file containing an array of tasks")

	listCmd := &cobra.Command{
		Use:   cmdListUse,
		Short: "List jobs",
		Long:  `List jobs, optionally filtered by status (pending, running, completed, or failed).`,
		Run: func(cmd *cobra.Command, args []string) {
			status, err := cmd.Flags().GetString("status")
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}

			page, err := psdk.ListJobs(defOffset, defLimit, status)
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}
			logJSONCmd(*cmd, page)
		},
	}
	listCmd.Flags().String("status", "", "Filter by job status (pending, running, completed, or failed)")

	viewCmd := &cobra.Command{
		Use:   cmdViewUse,
		Short: "View job",
		Long:  `View a single job.`,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) != 1 {
				logUsageCmd(*cmd, cmd.Use)

				return
			}

			job, err := psdk.GetJob(args[0])
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}
			logJSONCmd(*cmd, job)
		},
	}

	startCmd := &cobra.Command{
		Use:   cmdStartUse,
		Short: "Start job",
		Long:  `Start a job.`,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) != 1 {
				logUsageCmd(*cmd, cmd.Use)

				return
			}

			if err := psdk.StartJob(args[0]); err != nil {
				logErrorCmd(*cmd, err)

				return
			}
			logOKCmd(*cmd)
		},
	}

	stopCmd := &cobra.Command{
		Use:   cmdStopUse,
		Short: "Stop job",
		Long:  `Stop a job.`,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) != 1 {
				logUsageCmd(*cmd, cmd.Use)

				return
			}

			if err := psdk.StopJob(args[0]); err != nil {
				logErrorCmd(*cmd, err)

				return
			}
			logOKCmd(*cmd)
		},
	}

	cmd.AddCommand(createCmd)
	cmd.AddCommand(listCmd)
	cmd.AddCommand(viewCmd)
	cmd.AddCommand(startCmd)
	cmd.AddCommand(stopCmd)

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

func tasksFromFlags(cmd *cobra.Command) ([]sdk.Task, error) {
	inline, err := cmd.Flags().GetString("tasks")
	if err != nil {
		return nil, err
	}

	filePath, err := cmd.Flags().GetString("tasks-file")
	if err != nil {
		return nil, err
	}

	var data []byte
	switch {
	case inline != "" && filePath != "":
		return nil, errTasksConflict
	case inline != "":
		data = []byte(inline)
	case filePath != "":
		data, err = os.ReadFile(filePath)
		if err != nil {
			return nil, err
		}
	default:
		return nil, nil
	}

	var tasks []sdk.Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, err
	}

	return tasks, nil
}
