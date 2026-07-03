package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	toml "github.com/pelletier/go-toml"
	"github.com/spf13/cobra"
)

var addPropletsCmd = &cobra.Command{
	Use:   "add-proplets",
	Short: "Add proplets to an existing provisioned setup",
	Long: `Add more proplets to an existing Propeller deployment without re-provisioning from scratch.

Reads tenant_id, channel_id, and the current proplet count from the existing config file,
then creates new Atom entities (kind: service, profile: Service Account), API keys,
connects them to the channel, and appends their credentials to that file.

Example:
  propeller-cli provision add-proplets
  propeller-cli provision add-proplets -f /path/to/config.toml`,
	Run: func(cmd *cobra.Command, args []string) {
		tenantID, channelID, numExisting, err := readExistingConfig(fileName)
		if err != nil {
			logErrorCmd(*cmd, fmt.Errorf("failed to read %s: %w", fileName, err))

			return
		}

		var (
			identifier  string
			secret      string
			token       string
			numNewStr   string
			numNew      int
			newProplets []propletCreds
		)

		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Enter your Atom username").
					Value(&identifier).
					Validate(func(str string) error {
						if str == "" {
							return fmt.Errorf("username is required")
						}

						return nil
					}),
				huh.NewInput().
					Title("Enter your password").
					EchoMode(huh.EchoModePassword).
					Value(&secret).
					Validate(func(str string) error {
						if str == "" {
							return fmt.Errorf("password is required")
						}

						var err error
						token, err = atomSDK.Login(cmd.Context(), identifier, secret)
						if err != nil {
							return fmt.Errorf("%w: %w", errFailedToCreateToken, err)
						}

						return nil
					}),
			),
			huh.NewGroup(
				huh.NewInput().
					Title(fmt.Sprintf("How many proplets to add? (currently %d)", numExisting)).
					Value(&numNewStr).
					Validate(func(str string) error {
						switch str {
						case "":
							numNew = 1
						default:
							var err error
							numNew, err = strconv.Atoi(str)
							if err != nil || numNew < 1 {
								return fmt.Errorf("number of proplets must be a positive integer")
							}
						}

						newProplets = make([]propletCreds, numNew)
						for i := range numNew {
							pn := namegen.Generate()
							eid, err := atomSDK.CreateServiceEntity(cmd.Context(), pn, tenantID, token)
							if err != nil {
								return fmt.Errorf("%w: %w", errFailedEntityCreation, err)
							}
							key, err := atomSDK.CreateAPIKey(cmd.Context(), eid, "proplet-mqtt", token)
							if err != nil {
								return fmt.Errorf("%w: %w", errFailedEntityCreation, err)
							}
							newProplets[i] = propletCreds{EntityID: eid, APIKey: key}

							if err := atomSDK.Connect(cmd.Context(), eid, channelID, tenantID, token); err != nil {
								return fmt.Errorf("%w: %w", errFailedConnectionCreation, err)
							}
						}

						return nil
					}),
			),
		)

		if err := form.Run(); err != nil {
			logErrorCmd(*cmd, err)

			return
		}

		var newSections strings.Builder
		for i, pc := range newProplets {
			sectionIndex := numExisting + i + 1
			fmt.Fprintf(&newSections, `
[proplet%d]
tenant_id = "%s"
entity_id = "%s"
api_key = "%s"
channel_id = "%s"
`,
				sectionIndex,
				tenantID,
				pc.EntityID,
				pc.APIKey,
				channelID,
			)
		}

		existing, err := os.ReadFile(fileName)
		if err != nil {
			logErrorCmd(*cmd, fmt.Errorf("failed to read %s: %w", fileName, err))

			return
		}

		content := string(existing)
		if idx := strings.Index(content, "\n[proxy]"); idx != -1 {
			content = content[:idx] + newSections.String() + content[idx:]
		} else {
			content += newSections.String()
		}

		if err := os.WriteFile(fileName, []byte(content), filePermission); err != nil {
			logErrorCmd(*cmd, fmt.Errorf("failed to write %s: %w", fileName, err))

			return
		}

		logSuccessCmd(*cmd, fmt.Sprintf("Added %d proplet(s) to %s (total: %d)", numNew, fileName, numExisting+numNew))
	},
}

func readExistingConfig(path string) (tenantID, channelID string, numExisting int, err error) {
	tree, err := toml.LoadFile(path)
	if err != nil {
		return "", "", 0, err
	}

	manager, ok := tree.Get("manager").(*toml.Tree)
	if !ok {
		return "", "", 0, fmt.Errorf("missing [manager] section in config file")
	}
	tenantID, _ = manager.Get("tenant_id").(string)
	channelID, _ = manager.Get("channel_id").(string)
	if tenantID == "" || channelID == "" {
		return "", "", 0, fmt.Errorf("tenant_id and channel_id are required in [manager] section")
	}

	numExisting = countExistingProplets(tree)

	return tenantID, channelID, numExisting, nil
}

func countExistingProplets(tree *toml.Tree) int {
	maxIndex := 0
	for _, key := range tree.Keys() {
		var n int
		if _, scanErr := fmt.Sscanf(key, "proplet%d", &n); scanErr == nil {
			if n > maxIndex {
				maxIndex = n
			}
		}
	}
	if maxIndex > 0 {
		return maxIndex
	}
	if tree.Has("proplet") {
		return 1
	}

	return 0
}
