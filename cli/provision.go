package cli

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/0x6flab/namegenerator"
	"github.com/absmach/propeller/pkg/atomsdk"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

var (
	errFailedToCreateToken      = errors.New("failed to create access token")
	errFailedToCreateTenant     = errors.New("failed to create tenant")
	errFailedChannelCreation    = errors.New("failed to create channel")
	errFailedEntityCreation     = errors.New("failed to create entity")
	errFailedConnectionCreation = errors.New("failed to create connection")

	atomSDK  atomsdk.SDK
	namegen  = namegenerator.NewGenerator()
	fileName = "config.toml"
)

const filePermission = 0o600

func SetAtomSDK(s atomsdk.SDK) {
	atomSDK = s
}

var provisionCmd = &cobra.Command{
	Use:   "provision",
	Short: "Provision resources",
	Long:  `Provision necessary Atom resources for Propeller operation.`,
	Run: func(cmd *cobra.Command, args []string) {
		var (
			identifier      string
			secret          string
			token           string
			tenantName      string
			tenantID        string
			managerName     string
			managerEntityID string
			managerAPIKey   string
			numPropletsStr  string
			numProplets     int
			propletEntities []propletCreds
			proxyName       string
			proxyEntityID   string
			proxyAPIKey     string
			channelName     string
			channelID       string
		)

		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Enter your Atom username (or email)?").
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
					Title("Enter tenant name (leave empty to auto generate)").
					Value(&tenantName).
					Validate(func(str string) error {
						if str == "" {
							tenantName = namegen.Generate()
						}

						var err error
						tenantID, err = atomSDK.EnsureTenant(cmd.Context(), tenantName, token)
						if err != nil {
							return fmt.Errorf("%w: %w", errFailedToCreateTenant, err)
						}

						return nil
					}),
			),
			huh.NewGroup(
				huh.NewInput().
					Title("Enter manager entity name (leave empty to auto generate)").
					Value(&managerName).
					Validate(func(str string) error {
						if str == "" {
							managerName = namegen.Generate()
						}

						var err error
						managerEntityID, err = atomSDK.CreateServiceEntity(cmd.Context(), managerName, tenantID, token)
						if err != nil {
							return fmt.Errorf("%w: %w", errFailedEntityCreation, err)
						}
						managerAPIKey, err = atomSDK.CreateAPIKey(cmd.Context(), managerEntityID, "manager-mqtt", token)
						if err != nil {
							return fmt.Errorf("%w: %w", errFailedEntityCreation, err)
						}

						return nil
					}),
			),
			huh.NewGroup(
				huh.NewInput().
					Title("Enter number of proplets to create (default: 1)").
					Value(&numPropletsStr).
					Validate(func(str string) error {
						switch str {
						case "":
							numProplets = 1
						default:
							var err error
							numProplets, err = strconv.Atoi(str)
							if err != nil || numProplets < 1 {
								return errors.New("number of proplets must be a positive integer")
							}
						}

						propletEntities = make([]propletCreds, numProplets)
						for i := range numProplets {
							pn := namegen.Generate()
							eid, err := atomSDK.CreateServiceEntity(cmd.Context(), pn, tenantID, token)
							if err != nil {
								return fmt.Errorf("%w: %w", errFailedEntityCreation, err)
							}
							key, err := atomSDK.CreateAPIKey(cmd.Context(), eid, "proplet-mqtt", token)
							if err != nil {
								return fmt.Errorf("%w: %w", errFailedEntityCreation, err)
							}
							propletEntities[i] = propletCreds{EntityID: eid, APIKey: key}
						}

						return nil
					}),
			), huh.NewGroup(
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
							return fmt.Errorf("%w: %w", errFailedEntityCreation, err)
						}

						return nil
					}),
			), huh.NewGroup(
				huh.NewInput().
					Title("Enter channel name (leave empty to auto generate)").
					Value(&channelName).
					Validate(func(str string) error {
						if str == "" {
							channelName = namegen.Generate()
						}

						var err error
						channelID, err = atomSDK.CreateResource(cmd.Context(), channelName, tenantID, token)
						if err != nil {
							return fmt.Errorf("%w: %w", errFailedChannelCreation, err)
						}

						for _, pc := range append([]propletCreds{
							{EntityID: managerEntityID},
							{EntityID: proxyEntityID},
						}, propletEntities...) {
							if err := atomSDK.Connect(cmd.Context(), pc.EntityID, channelID, tenantID, token); err != nil {
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

		var configContent strings.Builder
		fmt.Fprintf(&configContent, `# Propeller Configuration
# Each identity is an Atom entity of kind "service", profile "Service Account".

[manager]
tenant_id = "%s"
entity_id = "%s"
api_key = "%s"
channel_id = "%s"
`,
			tenantID,
			managerEntityID,
			managerAPIKey,
			channelID,
		)

		for i, pc := range propletEntities {
			var sectionName string
			switch len(propletEntities) {
			case 1:
				sectionName = "[proplet]"
			default:
				sectionName = fmt.Sprintf("[proplet%d]", i+1)
			}

			fmt.Fprintf(&configContent, `
%s
tenant_id = "%s"
entity_id = "%s"
api_key = "%s"
channel_id = "%s"
`,
				sectionName,
				tenantID,
				pc.EntityID,
				pc.APIKey,
				channelID,
			)
		}

		fmt.Fprintf(&configContent, `
[proxy]
tenant_id = "%s"
entity_id = "%s"
api_key = "%s"
channel_id = "%s"`,
			tenantID,
			proxyEntityID,
			proxyAPIKey,
			channelID,
		)
		configContent.WriteString("\n")

		if err := os.WriteFile(fileName, []byte(configContent.String()), filePermission); err != nil {
			logErrorCmd(*cmd, fmt.Errorf("failed to create %s file: %w", fileName, err))

			return
		}

		logSuccessCmd(*cmd, fmt.Sprintf("Successfully created %s file", fileName))
	},
}

type propletCreds struct {
	EntityID string
	APIKey   string
}

func NewProvisionCmd() *cobra.Command {
	provisionCmd.PersistentFlags().StringVarP(
		&fileName,
		"file-name",
		"f",
		fileName,
		"The name of the file to create",
	)

	provisionCmd.AddCommand(addPropletsCmd)
	provisionCmd.AddCommand(addProxyCmd)

	return provisionCmd
}
