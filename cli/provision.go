package cli

import (
	"context"
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
	errFailedSharedKeyCreation  = errors.New("failed to create shared key")
	errFailedConnectionCreation = errors.New("failed to create connection")

	atomSDK  atomsdk.SDK
	namegen  = namegenerator.NewGenerator()
	fileName = "config.toml"
)

// Environment variables that let `provision` run without the interactive form,
// for CI and other unattended callers. Setting both credentials switches the
// command to the non-interactive path; every other input is optional and falls
// back to the same auto-generated name the form would have used.
const (
	envAtomIdentifier = "PROPELLER_ATOM_IDENTIFIER"
	envAtomSecret     = "PROPELLER_ATOM_SECRET"
	envTenantName     = "PROPELLER_TENANT_NAME"
	envManagerName    = "PROPELLER_MANAGER_ENTITY_NAME"
	envPropletCount   = "PROPELLER_PROPLET_COUNT"
	envProxyName      = "PROPELLER_PROXY_ENTITY_NAME"
	envChannelName    = "PROPELLER_CHANNEL_NAME"
)

// filePermission is world-readable because config.toml is bind-mounted into
// the proplet container, which reads it as a fixed non-root user (uid 999)
// that will not match whoever ran `provision` on the host.
const filePermission = 0o644

func SetAtomSDK(s atomsdk.SDK) {
	atomSDK = s
}

// provisionInput is the resolved set of answers `provision` needs. The
// interactive form fills it field by field; the non-interactive path fills it
// from the environment. Either way provisionResources does the actual work, so
// both paths create identical Atom state.
type provisionInput struct {
	identifier  string
	secret      string
	tenantName  string
	managerName string
	numProplets int
	proxyName   string
	channelName string
}

// provisionResult carries the identifiers and keys written to config.toml.
type provisionResult struct {
	tenantID        string
	managerEntityID string
	managerAPIKey   string
	proplets        []propletCreds
	proxyEntityID   string
	proxyAPIKey     string
	channelID       string
}

var provisionCmd = &cobra.Command{
	Use:   "provision",
	Short: "Provision resources",
	Long: `Provision necessary Atom resources for Propeller operation.

Runs an interactive form by default. Set ` + envAtomIdentifier + ` and ` + envAtomSecret + ` to
provision non-interactively; see the PROPELLER_* variables for the
remaining optional inputs.`,
	Run: func(cmd *cobra.Command, args []string) {
		input, ok, err := provisioningInputFromEnv()
		if err != nil {
			logErrorCmd(*cmd, err)

			return
		}

		if !ok {
			if input, err = promptProvisioning(); err != nil {
				logErrorCmd(*cmd, fmt.Errorf("provisioning failed: %w", err))

				return
			}
		}

		result, err := provisionResources(cmd.Context(), input)
		if err != nil {
			logErrorCmd(*cmd, fmt.Errorf("provisioning failed: %w", err))

			return
		}

		if err := writeConfigFile(fileName, []byte(renderConfig(result))); err != nil {
			logErrorCmd(*cmd, fmt.Errorf("%w: %s", errFailedToWriteConfig, err.Error()))

			return
		}

		logSuccessCmd(*cmd, fmt.Sprintf("Successfully created %s file", fileName))
	},
}

// provisioningInputFromEnv resolves the provisioning inputs from the
// environment. ok is false when no credentials are configured, signalling the
// caller to fall back to the interactive form.
func provisioningInputFromEnv() (provisionInput, bool, error) {
	var input provisionInput

	identifier, secret := os.Getenv(envAtomIdentifier), os.Getenv(envAtomSecret)

	switch {
	case identifier == "" && secret == "":
		return input, false, nil
	case identifier == "" || secret == "":
		return input, false, fmt.Errorf(
			"both %s and %s must be set to provision non-interactively", envAtomIdentifier, envAtomSecret)
	}

	input = provisionInput{
		identifier:  identifier,
		secret:      secret,
		tenantName:  os.Getenv(envTenantName),
		managerName: os.Getenv(envManagerName),
		proxyName:   os.Getenv(envProxyName),
		channelName: os.Getenv(envChannelName),
		numProplets: 1,
	}

	if raw := os.Getenv(envPropletCount); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return provisionInput{}, false, fmt.Errorf("%s must be a positive integer", envPropletCount)
		}

		input.numProplets = n
	}

	return input, true, nil
}

// promptProvisioning collects the provisioning inputs interactively. The
// fields only collect and validate values — the Atom calls happen once, in
// provisionResources, after the form is submitted.
func promptProvisioning() (provisionInput, error) {
	var (
		input          provisionInput
		numPropletsStr string
	)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Enter your Atom username (or email)?").
				Value(&input.identifier).
				Validate(func(str string) error {
					if str == "" {
						return errors.New("username is required")
					}

					return nil
				}),
			huh.NewInput().
				Title("Enter your password").
				EchoMode(huh.EchoModePassword).
				Value(&input.secret).
				Validate(func(str string) error {
					if str == "" {
						return errors.New("password is required")
					}

					return nil
				}),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Enter tenant name (leave empty to auto generate)").
				Value(&input.tenantName),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Enter manager entity name (leave empty to auto generate)").
				Value(&input.managerName),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Enter number of proplets to create (default: 1)").
				Value(&numPropletsStr).
				Validate(func(str string) error {
					if str == "" {
						input.numProplets = 1

						return nil
					}

					n, err := strconv.Atoi(str)
					if err != nil || n < 1 {
						return errors.New("number of proplets must be a positive integer")
					}

					input.numProplets = n

					return nil
				}),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Enter proxy entity name (leave empty to auto generate)").
				Value(&input.proxyName),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Enter channel name (leave empty to auto generate)").
				Value(&input.channelName),
		),
	)

	if err := form.Run(); err != nil {
		return input, err
	}

	return input, nil
}

// provisionResources creates the tenant, the manager/proplet/proxy service
// entities with their shared keys, the channel, and the entity↔channel
// connections. Empty names are auto-generated, matching the form's "leave
// empty to auto generate" prompts.
func provisionResources(ctx context.Context, input provisionInput) (provisionResult, error) {
	var result provisionResult

	token, err := atomSDK.Login(ctx, input.identifier, input.secret)
	if err != nil {
		return result, fmt.Errorf("%w: %s", errFailedToCreateToken, err.Error())
	}

	if input.tenantName == "" {
		input.tenantName = namegen.Generate()
	}

	result.tenantID, err = atomSDK.EnsureTenant(ctx, input.tenantName, token)
	if err != nil {
		return result, fmt.Errorf("%w: %s", errFailedToCreateTenant, err.Error())
	}

	if input.managerName == "" {
		input.managerName = namegen.Generate()
	}

	result.managerEntityID, err = atomSDK.CreateServiceEntity(ctx, input.managerName, result.tenantID, token)
	if err != nil {
		return result, fmt.Errorf("%w: %s", errFailedEntityCreation, err.Error())
	}

	result.managerAPIKey, err = atomSDK.CreateSharedKey(ctx, result.managerEntityID, "manager-mqtt", token)
	if err != nil {
		return result, fmt.Errorf("%w: %s", errFailedSharedKeyCreation, err.Error())
	}

	if input.numProplets < 1 {
		input.numProplets = 1
	}

	result.proplets = make([]propletCreds, input.numProplets)
	for i := range input.numProplets {
		eid, err := atomSDK.CreateServiceEntity(ctx, namegen.Generate(), result.tenantID, token)
		if err != nil {
			return result, fmt.Errorf("%w: %s", errFailedEntityCreation, err.Error())
		}

		key, err := atomSDK.CreateSharedKey(ctx, eid, "proplet-mqtt", token)
		if err != nil {
			return result, fmt.Errorf("%w: %s", errFailedSharedKeyCreation, err.Error())
		}

		result.proplets[i] = propletCreds{EntityID: eid, APIKey: key}
	}

	if input.proxyName == "" {
		input.proxyName = namegen.Generate()
	}

	result.proxyEntityID, err = atomSDK.CreateServiceEntity(ctx, input.proxyName, result.tenantID, token)
	if err != nil {
		return result, fmt.Errorf("%w: %s", errFailedEntityCreation, err.Error())
	}

	result.proxyAPIKey, err = atomSDK.CreateSharedKey(ctx, result.proxyEntityID, "proxy-mqtt", token)
	if err != nil {
		return result, fmt.Errorf("%w: %s", errFailedSharedKeyCreation, err.Error())
	}

	if input.channelName == "" {
		input.channelName = namegen.Generate()
	}

	result.channelID, err = atomSDK.CreateResource(ctx, input.channelName, result.tenantID, token)
	if err != nil {
		return result, fmt.Errorf("%w: %s", errFailedChannelCreation, err.Error())
	}

	for _, pc := range append([]propletCreds{
		{EntityID: result.managerEntityID},
		{EntityID: result.proxyEntityID},
	}, result.proplets...) {
		if err := atomSDK.Connect(ctx, pc.EntityID, result.channelID, result.tenantID, token); err != nil {
			return result, fmt.Errorf("%w: %s", errFailedConnectionCreation, err.Error())
		}
	}

	return result, nil
}

// renderConfig builds the config.toml contents from a provisioning result. A
// single proplet gets the plain [proplet] section name; multiple proplets are
// numbered from 1.
func renderConfig(result provisionResult) string {
	var configContent strings.Builder

	fmt.Fprintf(&configContent, `# Propeller Configuration
# Each identity is an Atom entity of kind "service", profile "Service Account".

[manager]
tenant_id = "%s"
entity_id = "%s"
api_key = "%s"
channel_id = "%s"
`,
		result.tenantID,
		result.managerEntityID,
		result.managerAPIKey,
		result.channelID,
	)

	for i, pc := range result.proplets {
		sectionName := "[proplet]"
		if len(result.proplets) > 1 {
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
			result.tenantID,
			pc.EntityID,
			pc.APIKey,
			result.channelID,
		)
	}

	fmt.Fprintf(&configContent, `
[proxy]
tenant_id = "%s"
entity_id = "%s"
api_key = "%s"
channel_id = "%s"
`,
		result.tenantID,
		result.proxyEntityID,
		result.proxyAPIKey,
		result.channelID,
	)

	return configContent.String()
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
