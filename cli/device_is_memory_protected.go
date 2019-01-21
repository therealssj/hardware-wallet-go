package cli

import (
	"fmt"

	gcli "github.com/urfave/cli"

	deviceWallet "github.com/skycoin/hardware-wallet-go/device-wallet"
)

func deviceIsMemoryProtected() gcli.Command {
	name := "deviceIsMemoryProtected"
	return gcli.Command{
		Name:         name,
		Usage:        "Ask if memory protection is enabled (bootloader mode only).",
		Description:  "",
		OnUsageError: onCommandUsageError(name),
		Action: func(c *gcli.Context) {
			isMemoryProtected := deviceWallet.DeviceIsMemoryProtected(deviceWallet.DeviceTypeUsb)
			fmt.Printf("%s\n", isMemoryProtected)
		},
	}
}
