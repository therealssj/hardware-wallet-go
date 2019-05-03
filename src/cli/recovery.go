package cli

import (
	"fmt"
	"os"

	gcli "github.com/urfave/cli"

	messages "github.com/skycoin/hardware-wallet-protob/go"

	deviceWallet "github.com/skycoin/hardware-wallet-go/src/device-wallet"
)

func recoveryCmd() gcli.Command {
	name := "recovery"
	return gcli.Command{
		Name:        name,
		Usage:       "Ask the device to perform the seed recovery procedure.",
		Description: "",
		Flags: []gcli.Flag{
			gcli.BoolFlag{
				Name:  "usePassphrase",
				Usage: "Configure a passphrase",
			},
			gcli.BoolFlag{
				Name:  "dryRun",
				Usage: "perform dry-run recovery workflow (for safe mnemonic validation)",
			},
			gcli.IntFlag{
				Name:  "wordCount",
				Usage: "Use a specific (12 | 24) number of words for the Mnemonic recovery",
				Value: 12,
			},
			gcli.StringFlag{
				Name:   "deviceType",
				Usage:  "Device type to send instructions to, hardware wallet (USB) or emulator.",
				EnvVar: "DEVICE_TYPE",
			},
		},
		OnUsageError: onCommandUsageError(name),
		Action: func(c *gcli.Context) {
			device := deviceWallet.NewDevice(deviceWallet.DeviceTypeFromString(c.String("deviceType")))
			if device == nil {
				return
			}

			if os.Getenv("AUTO_PRESS_BUTTONS") == "1" && device.Driver.DeviceType() == deviceWallet.DeviceTypeEmulator && runtime.GOOS == "linux" {
				err := device.SetAutoPressButton(true, deviceWallet.ButtonRight)
				if err != nil {
					log.Error(err)
					return
				}
			}

			passphrase := c.Bool("usePassphrase")
			dryRun := c.Bool("dryRun")
			wordCount := uint32(c.Uint64("wordCount"))
			msg, err := device.Recovery(wordCount, passphrase, dryRun)
			if err != nil {
				log.Error(err)
				return
			}

			for msg.Kind == uint16(messages.MessageType_MessageType_WordRequest) {
				var word string
				fmt.Printf("Word: ")
				fmt.Scanln(&word)
				msg, err = device.WordAck(word)
				if err != nil {
					log.Error(err.Error())
					os.Exit(1)
					return
				}
			}

			if msg.Kind == uint16(messages.MessageType_MessageType_ButtonRequest) {
				// Send ButtonAck
				msg, err = device.ButtonAck()
				if err != nil {
					log.Error(err)
					return
				}
			}

			responseMsg, err := deviceWallet.DecodeSuccessOrFailMsg(msg)
			if err != nil {
				log.Error(err)
				return
			}

			fmt.Println(responseMsg)
		},
	}
}
