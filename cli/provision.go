package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/0x6flab/namegenerator"
	"github.com/absmach/supermq/pkg/errors"
	smqSDK "github.com/absmach/supermq/pkg/sdk"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

var (
	errFailedToCreateToken      = errors.New("failed to create access token")
	errFailedToCreateDomain     = errors.New("failed to create domain")
	errFailedChannelCreation    = errors.New("failed to create channel")
	errFailedClientCreation     = errors.New("failed to create client")
	errFailedConnectionCreation = errors.New("failed to create connection")

	smqsdk   smqSDK.SDK
	namegen  = namegenerator.NewGenerator()
	fileName = "config.toml"
)

const filePermission = 0o644

func SetSuperMQSDK(sdk smqSDK.SDK) {
	smqsdk = sdk
}

var provisionCmd = &cobra.Command{
	Use:   "provision",
	Short: "Provision resources",
	Long:  `Provision necessary resources for Propeller operation.`,
	Run: func(cmd *cobra.Command, args []string) {
		var (
			username           string
			password           string
			err                error
			token              smqSDK.Token
			domainName         string
			domainAlias        string
			domainPermission   string
			domain             smqSDK.Domain
			managerClientName  string
			managerClient      smqSDK.Client
			propletClientName  string
			propletClient      smqSDK.Client
			managerChannelName string
			managerChannel     smqSDK.Channel
		)
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Enter your username?").
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

						token, err = smqsdk.CreateToken(u)
						if err != nil {
							return errors.Wrap(errFailedToCreateToken, err)
						}

						return nil
					}),
			),
			huh.NewGroup(
				huh.NewInput().
					Title("Enter your domain name(leave empty to auto generate)").
					Value(&domainName),
				huh.NewInput().
					Title("Enter your domain alias(leave empty to auto generate)").
					Value(&domainAlias),
				huh.NewSelect[string]().
					Title("Select your domain permission").
					Options(
						huh.NewOption("admin", "admin"),
						huh.NewOption("edit", "edit"),
						huh.NewOption("view", "view"),
					).
					Value(&domainPermission).
					Validate(func(str string) error {
						if domainName == "" {
							domainName = namegen.Generate()
						}
						if domainAlias == "" {
							domainAlias = strings.ToLower(domainName)
						}
						domain = smqSDK.Domain{
							Name:       domainName,
							Alias:      domainAlias,
							Permission: domainPermission,
						}
						domain, err = smqsdk.CreateDomain(domain, token.AccessToken)
						if err != nil {
							return errors.Wrap(errFailedToCreateDomain, err)
						}

						return nil
					}),
			),
			huh.NewGroup(
				huh.NewInput().
					Title("Enter your manager client name(leave empty to auto generate)").
					Value(&managerClientName).
					Validate(func(str string) error {
						if str == "" {
							managerClientName = namegen.Generate()
						}
						managerClient = smqSDK.Client{
							Name:   managerClientName,
							Tags:   []string{"manager", "propeller"},
							Status: "enabled",
						}
						managerClient, err = smqsdk.CreateClient(managerClient, domain.ID, token.AccessToken)
						if err != nil {
							return errors.Wrap(errFailedClientCreation, err)
						}

						return nil
					}),
			),
			huh.NewGroup(
				huh.NewInput().
					Title("Enter your proplet client name(leave empty to auto generate)").
					Value(&propletClientName).
					Validate(func(str string) error {
						if str == "" {
							propletClientName = namegen.Generate()
						}
						propletClient = smqSDK.Client{
							Name:   propletClientName,
							Tags:   []string{"proplet", "propeller"},
							Status: "enabled",
						}
						propletClient, err = smqsdk.CreateClient(propletClient, domain.ID, token.AccessToken)
						if err != nil {
							return errors.Wrap(errFailedClientCreation, err)
						}

						return nil
					}),
			), huh.NewGroup(
				huh.NewInput().
					Title("Enter your manager channel name(leave empty to auto generate)").
					Value(&managerChannelName).
					Validate(func(str string) error {
						if str == "" {
							managerChannelName = namegen.Generate()
						}
						managerChannel = smqSDK.Channel{
							Name:   managerChannelName,
							Status: "enabled",
						}
						managerChannel, err = smqsdk.CreateChannel(managerChannel, domain.ID, token.AccessToken)
						if err != nil {
							return errors.Wrap(errFailedChannelCreation, err)
						}

						managerConns := smqSDK.Connection{
							ClientIDs:  []string{managerClient.ID, propletClient.ID},
							ChannelIDs: []string{managerChannel.ID},
							Types:      []string{"publish", "subscribe"},
						}
						if err = smqsdk.Connect(managerConns, domain.ID, token.AccessToken); err != nil {
							return errors.Wrap(errFailedConnectionCreation, err)
						}

						return nil
					}),
			),
		)

		if err := form.Run(); err != nil {
			logErrorCmd(*cmd, errors.Wrap(errFailedConnectionCreation, err))

			return
		}

		configContent := fmt.Sprintf(`# SuperMQ Configuration

[manager]
client_id = "%s"
client_key = "%s"
channel_id = "%s"

[proplet]
client_id = "%s"
client_key = "%s"
channel_id = "%s"

[proxy]
client_id = "%s"
client_key = "%s"
channel_id = "%s"`,
			managerClient.ID,
			managerClient.Credentials.Secret,
			managerChannel.ID,
			propletClient.ID,
			propletClient.Credentials.Secret,
			managerChannel.ID,
			propletClient.ID,
			propletClient.Credentials.Secret,
			managerChannel.ID,
		)

		if err := os.WriteFile(fileName, []byte(configContent), filePermission); err != nil {
			logErrorCmd(*cmd, errors.New(fmt.Sprintf("failed to create %s file", fileName)))

			return
		}

		logSuccessCmd(*cmd, fmt.Sprintf("Successfully created %s file", fileName))
	},
}

func NewProvisionCmd() *cobra.Command {
	provisionCmd.PersistentFlags().StringVarP(
		&fileName,
		"file-name",
		"f",
		fileName,
		"The name of the file to create",
	)

	return provisionCmd
}
