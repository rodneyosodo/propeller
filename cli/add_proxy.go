package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

var addProxyCmd = &cobra.Command{
	Use:   "add-proxy",
	Short: "Add a proxy entity to an existing provisioned setup",
	Long: `Creates a new Atom service entity for the proxy, an API key, connects it to the channel,
and updates the config file.

Reads tenant_id and channel_id from the existing config file.

Example:
  propeller-cli provision add-proxy
  propeller-cli provision add-proxy -f /path/to/config.toml`,
	Run: func(cmd *cobra.Command, args []string) {
		tenantID, channelID, _, err := readExistingConfig(fileName)
		if err != nil {
			logErrorCmd(*cmd, fmt.Errorf("failed to read %s: %w", fileName, err))

			return
		}

		var (
			identifier    string
			secret        string
			token         string
			proxyName     string
			proxyEntityID string
			proxyAPIKey   string
		)

		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Enter your Atom username").
					Value(&identifier).
					Validate(func(str string) error {
						if str == "" {
							return errors.New("username is required")
						}

						return nil
					}),
				huh.NewInput().
					Title("Enter your password").
					EchoMode(huh.EchoModePassword).
					Value(&secret).
					Validate(func(str string) error {
						if str == "" {
							return errors.New("password is required")
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
					Title("Enter proxy entity name (leave empty to auto generate)").
					Value(&proxyName).
					Validate(func(str string) error {
						if str == "" {
							proxyName = namegen.Generate()
						}

						var err error
						proxyEntityID, err = atomSDK.CreateServiceEntity(cmd.Context(), proxyName, tenantID, token)
						if err != nil {
							return fmt.Errorf("%w: %w", errFailedEntityCreation, err)
						}
						proxyAPIKey, err = atomSDK.CreateAPIKey(cmd.Context(), proxyEntityID, "proxy-mqtt", token)
						if err != nil {
							return fmt.Errorf("%w: %w", errFailedAPIKeyCreation, err)
						}

						if err := atomSDK.Connect(cmd.Context(), proxyEntityID, channelID, tenantID, token); err != nil {
							return fmt.Errorf("%w: %w", errFailedConnectionCreation, err)
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
tenant_id = "%s"
entity_id = "%s"
api_key = "%s"
channel_id = "%s"
`,
			tenantID,
			proxyEntityID,
			proxyAPIKey,
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

		logSuccessCmd(*cmd, "Added proxy entity to "+fileName)
	},
}
