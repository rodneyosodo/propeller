package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/absmach/magistrala/pkg/errors"
	smqSDK "github.com/absmach/magistrala/pkg/sdk"
	"github.com/charmbracelet/huh"
	toml "github.com/pelletier/go-toml"
	"github.com/spf13/cobra"
)

var addPropletsCmd = &cobra.Command{
	Use:   "add-proplets",
	Short: "Add proplets to an existing provisioned setup",
	Long: `Add more proplets to an existing Propeller deployment without re-provisioning from scratch.

Reads domain_id, channel_id, and the current proplet count from the existing config file,
then creates new Magistrala clients and appends their credentials to that file.

Example:
  propeller-cli provision add-proplets
  propeller-cli provision add-proplets -f /path/to/config.toml`,
	Run: func(cmd *cobra.Command, args []string) {
		domainID, channelID, numExisting, err := readExistingConfig(fileName)
		if err != nil {
			logErrorCmd(*cmd, fmt.Errorf("failed to read %s: %w", fileName, err))

			return
		}

		var (
			username    string
			password    string
			token       smqSDK.Token
			numNewStr   string
			numNew      int
			newProplets []smqSDK.Client
		)

		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Enter your username").
					Value(&username).
					Validate(func(str string) error {
						if str == "" {
							return errors.New("username is required")
						}

						return nil
					}),
				huh.NewInput().
					Title("Enter your password").
					EchoMode(huh.EchoModePassword).
					Value(&password).
					Validate(func(str string) error {
						if str == "" {
							return errors.New("password is required")
						}
						u := smqSDK.Login{
							Username: username,
							Password: password,
						}
						token, err = smqsdk.CreateToken(cmd.Context(), u)
						if err != nil {
							return errors.Wrap(errFailedToCreateToken, err)
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
							numNew, err = strconv.Atoi(str)
							if err != nil || numNew < 1 {
								return errors.New("number of proplets must be a positive integer")
							}
						}

						newProplets = make([]smqSDK.Client, numNew)
						for i := range numNew {
							propletClientName := namegen.Generate()
							propletClient := smqSDK.Client{
								Name:   propletClientName,
								Tags:   []string{"proplet", "propeller"},
								Status: "enabled",
							}
							propletClient, err = smqsdk.CreateClient(cmd.Context(), propletClient, domainID, token.AccessToken)
							if err != nil {
								return errors.Wrap(errFailedClientCreation, err)
							}
							newProplets[i] = propletClient
						}

						clientIDs := make([]string, numNew)
						for i, p := range newProplets {
							clientIDs[i] = p.ID
						}
						conn := smqSDK.Connection{
							ClientIDs:  clientIDs,
							ChannelIDs: []string{channelID},
							Types:      []string{"publish", "subscribe"},
						}
						if err = smqsdk.Connect(cmd.Context(), conn, domainID, token.AccessToken); err != nil {
							return errors.Wrap(errFailedConnectionCreation, err)
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
		for i, propletClient := range newProplets {
			sectionIndex := numExisting + i + 1
			fmt.Fprintf(&newSections, `
[proplet%d]
domain_id = "%s"
client_id = "%s"
client_key = "%s"
channel_id = "%s"
`,
				sectionIndex,
				domainID,
				propletClient.ID,
				propletClient.Credentials.Secret,
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

func readExistingConfig(path string) (domainID, channelID string, numExisting int, err error) {
	tree, err := toml.LoadFile(path)
	if err != nil {
		return "", "", 0, err
	}

	manager, ok := tree.Get("manager").(*toml.Tree)
	if !ok {
		return "", "", 0, errors.New("missing [manager] section in config file")
	}
	domainID, _ = manager.Get("domain_id").(string)
	channelID, _ = manager.Get("channel_id").(string)
	if domainID == "" || channelID == "" {
		return "", "", 0, errors.New("domain_id and channel_id are required in [manager] section")
	}

	numExisting = countExistingProplets(tree)

	return domainID, channelID, numExisting, nil
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
