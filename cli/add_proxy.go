package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/absmach/magistrala/pkg/errors"
	smqSDK "github.com/absmach/magistrala/pkg/sdk"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

var addProxyCmd = &cobra.Command{
	Use:   "add-proxy",
	Short: "Add a proxy client to an existing provisioned setup",
	Long: `Creates a new Magistrala client for the proxy service and updates the config file.

Reads domain_id and channel_id from the existing config file,
creates a new client, connects it to the channel, and writes its credentials.

Example:
  propeller-cli provision add-proxy
  propeller-cli provision add-proxy -f /path/to/config.toml`,
	Run: func(cmd *cobra.Command, args []string) {
		domainID, channelID, _, err := readExistingConfig(fileName)
		if err != nil {
			logErrorCmd(*cmd, fmt.Errorf("failed to read %s: %w", fileName, err))

			return
		}

		var (
			username   string
			password   string
			token      smqSDK.Token
			proxyName  string
			proxyID    string
			proxyKey   string
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
					Title("Enter proxy client name (leave empty to auto generate)").
					Value(&proxyName).
					Validate(func(str string) error {
						if str == "" {
							proxyName = namegen.Generate()
						}
						proxyClient := smqSDK.Client{
							Name:   proxyName,
							Tags:   []string{"proxy", "propeller"},
							Status: "enabled",
						}
						created, err := smqsdk.CreateClient(cmd.Context(), proxyClient, domainID, token.AccessToken)
						if err != nil {
							return errors.Wrap(errFailedClientCreation, err)
						}
						proxyID = created.ID
						proxyKey = created.Credentials.Secret

						conn := smqSDK.Connection{
							ClientIDs:  []string{proxyID},
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

		existing, err := os.ReadFile(fileName)
		if err != nil {
			logErrorCmd(*cmd, fmt.Errorf("failed to read %s: %w", fileName, err))

			return
		}

		var newSection strings.Builder
		fmt.Fprintf(&newSection, `
[proxy]
domain_id = "%s"
client_id = "%s"
client_key = "%s"
channel_id = "%s"
`,
			domainID,
			proxyID,
			proxyKey,
			channelID,
		)

		content := string(existing)
		proxyMarker := "\n[proxy]"
		if idx := strings.Index(content, proxyMarker); idx != -1 {
			sectionEnd := strings.Index(content[idx+1:], "\n[")
			if sectionEnd != -1 {
				content = content[:idx] + content[idx+1+sectionEnd:]
			} else {
				content = content[:idx]
			}
		}
		content += newSection.String()

		if err := os.WriteFile(fileName, []byte(content), filePermission); err != nil {
			logErrorCmd(*cmd, fmt.Errorf("failed to write %s: %w", fileName, err))

			return
		}

		logSuccessCmd(*cmd, fmt.Sprintf("Added proxy client to %s", fileName))
	},
}
