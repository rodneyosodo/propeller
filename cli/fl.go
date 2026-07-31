package cli

import (
	"encoding/json"
	"os"

	"github.com/absmach/propeller/pkg/sdk"
	"github.com/spf13/cobra"
)

func NewFLCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fl [configure|task|update|update-cbor|round-status]",
		Short: "Federated learning manager",
		Long:  `Interact with the federated learning API.`,
	}

	cmd.AddCommand(newFLConfigureCmd())
	cmd.AddCommand(newFLTaskCmd())
	cmd.AddCommand(newFLUpdateCmd())
	cmd.AddCommand(newFLUpdateCBORCmd())
	cmd.AddCommand(newFLRoundStatusCmd())

	return cmd
}

func newFLConfigureCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Configure FL experiment",
		Long:  `Configure a new federated learning experiment.`,
		Run: func(cmd *cobra.Command, args []string) {
			experimentID, err := cmd.Flags().GetString("experiment-id")
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}
			if experimentID == "" {
				logErrorCmd(*cmd, errExperimentIDRequired)

				return
			}

			roundID, err := cmd.Flags().GetString("round-id")
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}

			modelRef, err := cmd.Flags().GetString("model-ref")
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}

			participants, err := cmd.Flags().GetStringSlice("participants")
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}

			hyperparams, err := jsonFlag(cmd, "hyperparams")
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}

			kOfN, err := cmd.Flags().GetInt("k-of-n")
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}

			timeoutS, err := cmd.Flags().GetInt("timeout-s")
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}

			taskWasmImage, err := cmd.Flags().GetString("task-wasm-image")
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}

			result, err := psdk.ConfigureExperiment(sdk.ExperimentConfig{
				ExperimentID:  experimentID,
				RoundID:       roundID,
				ModelRef:      modelRef,
				Participants:  participants,
				Hyperparams:   hyperparams,
				KOfN:          kOfN,
				TimeoutS:      timeoutS,
				TaskWasmImage: taskWasmImage,
			})
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}
			logJSONCmd(*cmd, result)
		},
	}
	cmd.Flags().String("experiment-id", "", "Unique experiment identifier")
	cmd.Flags().String("round-id", "", "Round identifier")
	cmd.Flags().String("model-ref", "", "Reference to the model")
	cmd.Flags().StringSlice("participants", nil, "Participant proplet IDs (comma-separated or repeatable)")
	cmd.Flags().String("hyperparams", "", "Hyperparameters as a JSON object")
	cmd.Flags().Int("k-of-n", 0, "Minimum updates required (k out of n participants)")
	cmd.Flags().Int("timeout-s", 0, "Timeout in seconds for each round")
	cmd.Flags().String("task-wasm-image", "", "OCI image URL for the FL task Wasm binary")

	return cmd
}

func newFLTaskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Get FL task",
		Long:  `Get the federated learning task for the current round.`,
		Run: func(cmd *cobra.Command, args []string) {
			roundID, err := cmd.Flags().GetString("round-id")
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}
			if roundID == "" {
				logErrorCmd(*cmd, errRoundIDRequired)

				return
			}

			propletID, err := cmd.Flags().GetString("proplet-id")
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}

			task, err := psdk.GetFLTask(roundID, propletID)
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}
			logJSONCmd(*cmd, task)
		},
	}
	cmd.Flags().String("round-id", "", "Round identifier")
	cmd.Flags().String("proplet-id", "", "Proplet ID requesting the task")

	return cmd
}

func newFLUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Submit FL update (JSON)",
		Long:  `Submit a model update in JSON format.`,
		Run: func(cmd *cobra.Command, args []string) {
			roundID, err := cmd.Flags().GetString("round-id")
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}
			if roundID == "" {
				logErrorCmd(*cmd, errRoundIDRequired)

				return
			}

			propletID, err := cmd.Flags().GetString("proplet-id")
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}

			baseModelURI, err := cmd.Flags().GetString("base-model-uri")
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}

			numSamples, err := cmd.Flags().GetInt("num-samples")
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}

			update, err := jsonFlag(cmd, "update")
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}

			metrics, err := jsonFlag(cmd, "metrics")
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}

			if err := psdk.PostFLUpdate(sdk.FLUpdate{
				RoundID:      roundID,
				PropletID:    propletID,
				BaseModelURI: baseModelURI,
				NumSamples:   numSamples,
				Metrics:      metrics,
				Update:       update,
			}); err != nil {
				logErrorCmd(*cmd, err)

				return
			}
			logOKCmd(*cmd)
		},
	}
	cmd.Flags().String("round-id", "", "Round identifier")
	cmd.Flags().String("proplet-id", "", "Proplet ID submitting the update")
	cmd.Flags().String("base-model-uri", "", "URI of the base model used for training")
	cmd.Flags().Int("num-samples", 0, "Number of samples used in local training")
	cmd.Flags().String("update", "", "Model weight updates as a JSON object")
	cmd.Flags().String("metrics", "", "Training metrics as a JSON object")

	return cmd
}

func newFLUpdateCBORCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update-cbor <file>",
		Short: "Submit FL update (CBOR)",
		Long:  `Submit a model update in CBOR or CBOR-seq format from a file.`,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) != 1 {
				logUsageCmd(*cmd, cmd.Use)

				return
			}

			data, err := os.ReadFile(args[0])
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}

			if err := psdk.PostFLUpdateCBOR(data); err != nil {
				logErrorCmd(*cmd, err)

				return
			}
			logOKCmd(*cmd)
		},
	}
}

func newFLRoundStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "round-status <round-id>",
		Short: "Get round completion status",
		Long:  `Check if a federated learning round is complete.`,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) != 1 {
				logUsageCmd(*cmd, cmd.Use)

				return
			}

			status, err := psdk.GetRoundStatus(args[0])
			if err != nil {
				logErrorCmd(*cmd, err)

				return
			}
			logJSONCmd(*cmd, status)
		},
	}
}

// jsonFlag returns the parsed JSON object of the given flag. An empty map is
// returned when the flag is not set.
func jsonFlag(cmd *cobra.Command, name string) (map[string]any, error) {
	raw, err := cmd.Flags().GetString(name)
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return map[string]any{}, nil
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, err
	}

	return m, nil
}
