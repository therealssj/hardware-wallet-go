package cli

import (
	"fmt"

	gcli "github.com/urfave/cli"

	messages "github.com/skycoin/hardware-wallet-protob/go"

	skyWallet "github.com/skycoin/hardware-wallet-go/src/skywallet"
)

func sandbox() gcli.Command {
	name := "sandbox"
	return gcli.Command{
		Name:         name,
		Usage:        "Sandbox.",
		Description:  "",
		Flags:        []gcli.Flag{},
		OnUsageError: onCommandUsageError(name),
		Action: func(c *gcli.Context) {
			device := skyWallet.NewDevice(skyWallet.DeviceTypeFromString(c.String("deviceType")))
			if device == nil {
				return
			}
			defer device.Close()

			_, err := device.Wipe()
			if err != nil {
				log.Error(err)
				return
			}

			_, err = device.SetMnemonic("cloud flower upset remain green metal below cup stem infant art thank")
			if err != nil {
				log.Error(err)
				return
			}

			var pinEnc string
			msg, err := device.ChangePin(nil)
			if err != nil {
				log.Error(err)
				return
			}

			for msg.Kind == uint16(messages.MessageType_MessageType_PinMatrixRequest) {
				log.Printf("PinMatrixRequest response: ")
				fmt.Scanln(&pinEnc)
				msg, err = device.PinMatrixAck(pinEnc)
				if err != nil {
					log.Error(err)
					return
				}
			}

			// come on one-more time
			// testing what happen when we try to change an existing pin code
			msg, err = device.ChangePin(nil)
			if err != nil {
				log.Error(err)
				return
			}

			for msg.Kind == uint16(messages.MessageType_MessageType_PinMatrixRequest) {
				log.Printf("PinMatrixRequest response: ")
				fmt.Scanln(&pinEnc)
				msg, err = device.PinMatrixAck(pinEnc)
				if err != nil {
					log.Error(err)
					return
				}
			}

			msg, err = device.AddressGen(9, 15, false)
			if err != nil {
				log.Error(err)
				return
			}

			if msg.Kind == uint16(messages.MessageType_MessageType_PinMatrixRequest) {
				log.Printf("PinMatrixRequest response: ")
				fmt.Scanln(&pinEnc)
				msg, err = device.PinMatrixAck(pinEnc)
				if err != nil {
					log.Error(err)
					return
				}

				if msg.Kind == uint16(messages.MessageType_MessageType_ResponseSkycoinAddress) {
					addresses, err := skyWallet.DecodeResponseSkycoinAddress(msg)
					if err != nil {
						log.Error(err)
						return
					}
					log.Print("Successfully got address")
					log.Print(addresses)
				}
			} else {
				log.Println("Got addresses without pin code")
				addresses, err := skyWallet.DecodeResponseSkycoinAddress(msg)
				if err != nil {
					log.Error(err)
					return
				}
				log.Print(addresses)
			}
		},
	}
}
