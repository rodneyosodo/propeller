package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/absmach/supermq/pkg/errors"
	smqSDK "github.com/absmach/supermq/pkg/sdk"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"go.bug.st/serial"
)

var addEmbedPropletCmd = &cobra.Command{
	Use:   "embed",
	Short: "Provision an embedded proplet over serial",
	Long: `Create a new SuperMQ client and write its credentials directly to an
embedded proplet (e.g. ESP32-S3 running Zephyr) via the serial shell.

Reads domain_id and channel_id from the existing config file, creates one
new SuperMQ client, then sends 'proplet creds set' commands over the
specified serial port.

Example:
  propeller-cli provision add-proplets embed
  propeller-cli provision add-proplets embed -f /path/to/config.toml`,
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
			wifiSSID   string
			wifiPSK    string
			serialPort string
			client     smqSDK.Client
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
						u := smqSDK.Login{Username: username, Password: password}
						token, err = smqsdk.CreateToken(cmd.Context(), u)
						if err != nil {
							return errors.Wrap(errFailedToCreateToken, err)
						}
						return nil
					}),
			),
			huh.NewGroup(
				huh.NewInput().
					Title("WiFi SSID for the embedded proplet").
					Value(&wifiSSID).
					Validate(func(str string) error {
						if str == "" {
							return errors.New("WiFi SSID is required")
						}
						return nil
					}),
				huh.NewInput().
					Title("WiFi password").
					EchoMode(huh.EchoModePassword).
					Value(&wifiPSK).
					Validate(func(str string) error {
						if str == "" {
							return errors.New("WiFi password is required")
						}

						propletClientName := namegen.Generate()
						propletClient := smqSDK.Client{
							Name:   propletClientName,
							Tags:   []string{"proplet", "propeller", "embedded"},
							Status: "enabled",
						}
						client, err = smqsdk.CreateClient(cmd.Context(), propletClient, domainID, token.AccessToken)
						if err != nil {
							return errors.Wrap(errFailedClientCreation, err)
						}
						conn := smqSDK.Connection{
							ClientIDs:  []string{client.ID},
							ChannelIDs: []string{channelID},
							Types:      []string{"publish", "subscribe"},
						}
						if err = smqsdk.Connect(cmd.Context(), conn, domainID, token.AccessToken); err != nil {
							return errors.Wrap(errFailedConnectionCreation, err)
						}
						return nil
					}),
				huh.NewInput().
					Title("Serial port (e.g. /dev/ttyUSB0)").
					Value(&serialPort).
					Placeholder("/dev/ttyUSB0").
					Validate(func(str string) error {
						if str == "" {
							serialPort = "/dev/ttyUSB0"
						}
						return nil
					}),
			),
		)

		if err := form.Run(); err != nil {
			logErrorCmd(*cmd, err)
			return
		}

		if err := provisionEmbedOverSerial(serialPort, wifiSSID, wifiPSK, client.ID, client.Credentials.Secret, domainID, channelID); err != nil {
			logErrorCmd(*cmd, fmt.Errorf("serial provisioning failed: %w", err))
			return
		}

		logSuccessCmd(*cmd, fmt.Sprintf(
			"Embedded proplet provisioned on %s (client_id: %s). Reboot the device to apply.",
			serialPort, client.ID,
		))
	},
}

func provisionEmbedOverSerial(port, wifiSSID, wifiPSK, propletID, clientKey, domainID, channelID string) error {
	mode := &serial.Mode{BaudRate: 115200}
	s, err := serial.Open(port, mode)
	if err != nil {
		return fmt.Errorf("open %s: %w", port, err)
	}
	defer s.Close()

	_ = s.SetDTR(false)
	_ = s.SetRTS(false)
	time.Sleep(3 * time.Second)

	fields := [][2]string{
		{"wifi_ssid", wifiSSID},
		{"wifi_psk", wifiPSK},
		{"proplet_id", propletID},
		{"client_key", clientKey},
		{"domain_id", domainID},
		{"channel_id", channelID},
	}

	for _, f := range fields {
		cmd := fmt.Sprintf("proplet creds set %s %s\r\n", f[0], f[1])
		if _, err := s.Write([]byte(cmd)); err != nil {
			return fmt.Errorf("write %s: %w", f[0], err)
		}

		resp, err := readSerialResponse(s, 3*time.Second)
		if err != nil {
			return fmt.Errorf("read response for %s: %w", f[0], err)
		}
		if !strings.Contains(resp, "Saved") {
			return fmt.Errorf("unexpected response for %s: %q", f[0], resp)
		}
	}

	return nil
}

func readSerialResponse(s serial.Port, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	var buf strings.Builder
	tmp := make([]byte, 128)

	for time.Now().Before(deadline) {
		n, err := s.Read(tmp)
		if err != nil {
			return buf.String(), err
		}
		if n > 0 {
			buf.Write(tmp[:n])
			if strings.Contains(buf.String(), "\n") {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	return buf.String(), nil
}

func init() {
	addPropletsCmd.AddCommand(addEmbedPropletCmd)
}
