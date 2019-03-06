package devicewallet

import (
	"io"
	"time"

	"github.com/skycoin/skycoin/src/util/logging"

	"github.com/gogo/protobuf/proto"

	"github.com/skycoin/hardware-wallet-go/src/device-wallet/messages"
	"github.com/skycoin/hardware-wallet-go/src/device-wallet/wire"
)

// DeviceType type of device: emulated or usb
type DeviceType int32

var (
	log = logging.MustGetLogger("device-wallet")
)

const (
	// DeviceTypeEmulator use emulator
	DeviceTypeEmulator DeviceType = 1
	// DeviceTypeUsb use usb
	DeviceTypeUsb DeviceType = 2
)

type DeviceWallet interface {
	AddressGen(deviceType DeviceType, addressN int, startIndex int, confirmAddress bool) (wire.Message, error)
	ApplySettings(deviceType DeviceType, usePassphrase bool, label string) (wire.Message, error)
	Backup(deviceType DeviceType) (wire.Message, error)
	Cancel(deviceType DeviceType) (wire.Message, error)
	CheckMessageSignature(deviceType DeviceType, message string, signature string, address string) (wire.Message, error)
	ChangePin(deviceType DeviceType) (wire.Message, error)
	Connected(deviceType DeviceType) bool
	FirmwareUpload(payload []byte, hash [32]byte) error
	GetFeatures(deviceType DeviceType) (wire.Message, error)
	GenerateMnemonic(deviceType DeviceType, wordCount uint32, usePassphrase bool) (wire.Message, error)
	Recovery(deviceType DeviceType, wordCount uint32, usePassphrase, dryRun bool) (wire.Message, error)
	SetMnemonic(deviceType DeviceType, mnemonic string) (wire.Message, error)
	TransactionSign(deviceType DeviceType, inputs []*messages.SkycoinTransactionInput, outputs []*messages.SkycoinTransactionOutput) (wire.Message, error)
	SignMessage(deviceType DeviceType, addressN int, message string) (wire.Message, error)
	Wipe(deviceType DeviceType) (wire.Message, error)
	ButtonAck(deviceType DeviceType) (wire.Message, error)
	PassphraseAck(deviceType DeviceType, passphrase string) (wire.Message, error)
	WordAck(deviceType DeviceType, word string) (wire.Message, error)
	PinMatrixAck(deviceType DeviceType, p string) (wire.Message, error)
}

// SkyWallet provides skycoin hardware wallet functions
type SkyWallet struct{}

// AddressGen Ask the device to generate an address
func (d *SkyWallet) AddressGen(deviceType DeviceType, addressN int, startIndex int, confirmAddress bool) (wire.Message, error) {
	dev, err := getDevice(deviceType)
	if err != nil {
		return wire.Message{}, err
	}
	defer dev.Close()
	skycoinAddress := &messages.SkycoinAddress{
		AddressN:       proto.Uint32(uint32(addressN)),
		ConfirmAddress: proto.Bool(confirmAddress),
		StartIndex:     proto.Uint32(uint32(startIndex)),
	}
	data, err := proto.Marshal(skycoinAddress)
	if err != nil {
		return wire.Message{}, err
	}

	chunks := makeTrezorMessage(data, messages.MessageType_MessageType_SkycoinAddress)

	return sendToDevice(dev, chunks)
}

// ApplySettings send ApplySettings request to the device
func (d *SkyWallet) ApplySettings(deviceType DeviceType, usePassphrase bool, label string) (wire.Message, error) {
	dev, err := getDevice(deviceType)
	if err != nil {
		return wire.Message{}, err
	}
	defer dev.Close()

	applySettings := &messages.ApplySettings{
		Label:         proto.String(label),
		Language:      proto.String(""),
		UsePassphrase: proto.Bool(usePassphrase),
	}
	log.Println(applySettings)
	data, err := proto.Marshal(applySettings)
	if err != nil {
		return wire.Message{}, err
	}

	chunks := makeTrezorMessage(data, messages.MessageType_MessageType_ApplySettings)
	return sendToDevice(dev, chunks)
}

// BackupDevice ask the device to perform the seed backup
func (d *SkyWallet) Backup(deviceType DeviceType) (wire.Message, error) {
	dev, err := getDevice(deviceType)
	if err != nil {
		return wire.Message{}, err
	}
	defer dev.Close()
	var msg wire.Message
	var chunks [][64]byte
	err = initialize(dev)
	if err != nil {
		return wire.Message{}, err
	}

	backupDevice := &messages.BackupDevice{}
	data, err := proto.Marshal(backupDevice)
	if err != nil {
		return wire.Message{}, err
	}
	chunks = makeTrezorMessage(data, messages.MessageType_MessageType_BackupDevice)
	msg, err = sendToDevice(dev, chunks)
	if err != nil {
		return wire.Message{}, err
	}

	for msg.Kind == uint16(messages.MessageType_MessageType_ButtonRequest) {
		msg, err = deviceButtonAck(dev)
		if err != nil {
			return wire.Message{}, err
		}
	}

	return msg, nil
}

// Cancel send Cancel request
func (d *SkyWallet) Cancel(deviceType DeviceType) (wire.Message, error) {
	dev, err := getDevice(deviceType)
	if err != nil {
		return wire.Message{}, err
	}
	defer dev.Close()

	chunks, err := MessageCancel()
	if err != nil {
		return wire.Message{}, err
	}

	return sendToDevice(dev, chunks)
}

// CheckMessageSignature Check a message signature matches the given address.
func (s *SkyWallet) CheckMessageSignature(deviceType DeviceType, message string, signature string, address string) (wire.Message, error) {
	dev, err := getDevice(deviceType)
	if err != nil {
		return wire.Message{}, err
	}
	defer dev.Close()

	// Send CheckMessageSignature

	skycoinCheckMessageSignature := &messages.SkycoinCheckMessageSignature{
		Address:   proto.String(address),
		Message:   proto.String(message),
		Signature: proto.String(signature),
	}

	data, err := proto.Marshal(skycoinCheckMessageSignature)
	if err != nil {
		return wire.Message{}, err
	}
	chunks := makeTrezorMessage(data, messages.MessageType_MessageType_SkycoinCheckMessageSignature)
	msg, err := sendToDevice(dev, chunks)
	if err != nil {
		return msg, err
	}
	log.Printf("Success %s! address that issued the signature is: %s\n", messages.MessageType(msg.Kind), msg.Data)
	return msg, nil
}

// ChangePin changes device's PIN code
// The message that is sent contains an encoded form of the PIN.
// The digits of the PIN are displayed in a 3x3 matrix on the Trezor,
// and the message that is sent back is a string containing the positions
// of the digits on that matrix. Below is the mapping between positions
// and characters to be sent:
// 7 8 9
// 4 5 6
// 1 2 3
// For example, if the numbers are laid out in this way on the Trezor,
// 3 1 5
// 7 8 4
// 9 6 2
// To set the PIN "12345", the positions are:
// top, bottom-right, top-left, right, top-right
// so you must send "83769".
func (d *SkyWallet) ChangePin(deviceType DeviceType) (wire.Message, error) {
	dev, err := getDevice(deviceType)
	if err != nil {
		return wire.Message{}, err
	}
	defer dev.Close()

	changePin := &messages.ChangePin{}
	data, _ := proto.Marshal(changePin)
	chunks := makeTrezorMessage(data, messages.MessageType_MessageType_ChangePin)
	msg, err := sendToDevice(dev, chunks)
	if err != nil {
		return wire.Message{}, err
	}

	// Acknowledge that a button has been pressed
	if msg.Kind == uint16(messages.MessageType_MessageType_ButtonRequest) {
		msg, err = deviceButtonAck(dev)
		if err != nil {
			return msg, err
		}
	}
	return msg, nil
}

// Connected check if a device is connected
func (d *SkyWallet) Connected(deviceType DeviceType) bool {
	dev, err := getDevice(deviceType)
	if dev == nil {
		return false
	}
	defer dev.Close()
	if err != nil {
		return false
	}
	msgRaw := &messages.Ping{}
	data, err := proto.Marshal(msgRaw)
	if err != nil {
		log.Print(err.Error())
	}
	chunks := makeTrezorMessage(data, messages.MessageType_MessageType_Ping)
	for _, element := range chunks {
		_, err = dev.Write(element[:])
		if err != nil {
			return false
		}
	}
	var msg wire.Message
	_, err = msg.ReadFrom(dev)
	if err != nil {
		return false
	}
	return msg.Kind == uint16(messages.MessageType_MessageType_Success)
}

// FirmwareUpload Updates device's firmware
func (d *SkyWallet) FirmwareUpload(payload []byte, hash [32]byte) error {
	dev, err := getDevice(DeviceTypeUsb)
	if err != nil {
		return err
	}
	defer dev.Close()

	err = initialize(dev)
	if err != nil {
		return err
	}

	log.Printf("Length of firmware %d", uint32(len(payload)))
	deviceFirmwareErase := &messages.FirmwareErase{
		Length: proto.Uint32(uint32(len(payload))),
	}

	erasedata, err := proto.Marshal(deviceFirmwareErase)
	if err != nil {
		return err
	}

	chunks := makeTrezorMessage(erasedata, messages.MessageType_MessageType_FirmwareErase)
	erasemsg, err := sendToDevice(dev, chunks)
	if err != nil {
		return err
	}
	log.Printf("Success %d! FirmwareErase %s\n", erasemsg.Kind, erasemsg.Data)

	log.Printf("Hash: %x\n", hash)
	deviceFirmwareUpload := &messages.FirmwareUpload{
		Payload: payload,
		Hash:    hash[:],
	}

	uploaddata, err := proto.Marshal(deviceFirmwareUpload)
	if err != nil {
		return err
	}
	chunks = makeTrezorMessage(uploaddata, messages.MessageType_MessageType_FirmwareUpload)

	uploadmsg, err := sendToDevice(dev, chunks)
	if err != nil {
		return err
	}
	log.Printf("Success %d! FirmwareUpload %s\n", uploadmsg.Kind, uploadmsg.Data)

	// Send ButtonAck
	chunks, err = MessageButtonAck()
	if err != nil {
		return err
	}
	return sendToDeviceNoAnswer(dev, chunks)
}

// GetFeatures send Features message to the device
func (d *SkyWallet) GetFeatures(deviceType DeviceType) (wire.Message, error) {
	dev, err := getDevice(deviceType)
	if err != nil {
		return wire.Message{}, err
	}
	defer dev.Close()

	featureMsg := &messages.GetFeatures{}
	data, err := proto.Marshal(featureMsg)
	if err != nil {
		return wire.Message{}, err
	}

	chunks := makeTrezorMessage(data, messages.MessageType_MessageType_GetFeatures)

	return sendToDevice(dev, chunks)
}

// GenerateMnemonic Ask the device to generate a mnemonic and configure itself with it.
func (d *SkyWallet) GenerateMnemonic(deviceType DeviceType, wordCount uint32, usePassphrase bool) (wire.Message, error) {
	dev, err := getDevice(deviceType)
	if err != nil {
		return wire.Message{}, err
	}
	defer dev.Close()

	skycoinGenerateMnemonic := &messages.GenerateMnemonic{
		PassphraseProtection: proto.Bool(usePassphrase),
		WordCount:            proto.Uint32(wordCount),
	}

	data, err := proto.Marshal(skycoinGenerateMnemonic)
	if err != nil {
		return wire.Message{}, err
	}

	chunks := makeTrezorMessage(data, messages.MessageType_MessageType_GenerateMnemonic)

	msg, err := sendToDevice(dev, chunks)
	if err != nil {
		return wire.Message{}, err
	}

	if msg.Kind == uint16(messages.MessageType_MessageType_ButtonRequest) {
		msg, err = deviceButtonAck(dev)
		if err != nil {
			return wire.Message{}, err
		}
	}

	return msg, err
}

// RecoveryDevice ask the device to perform the seed backup
func (d *SkyWallet) Recovery(deviceType DeviceType, wordCount uint32, usePassphrase, dryRun bool) (wire.Message, error) {
	dev, err := getDevice(deviceType)
	if err != nil {
		return wire.Message{}, err
	}
	defer dev.Close()
	var msg wire.Message
	var chunks [][64]byte

	log.Printf("Using passphrase %t\n", usePassphrase)

	recoveryDevice := &messages.RecoveryDevice{
		WordCount:            proto.Uint32(wordCount),
		PassphraseProtection: proto.Bool(usePassphrase),
		DryRun:               proto.Bool(dryRun),
	}
	data, err := proto.Marshal(recoveryDevice)
	if err != nil {
		return wire.Message{}, err
	}
	chunks = makeTrezorMessage(data, messages.MessageType_MessageType_RecoveryDevice)
	msg, err = sendToDevice(dev, chunks)
	if err != nil {
		return msg, err
	}
	log.Printf("Recovery device %d! Answer is: %s\n", msg.Kind, msg.Data)

	if msg.Kind == uint16(messages.MessageType_MessageType_ButtonRequest) {
		msg, err = deviceButtonAck(dev)
		if err != nil {
			return wire.Message{}, err
		}
	}

	return msg, nil
}

// SetMnemonic Configure the device with a mnemonic.
func (d *SkyWallet) SetMnemonic(deviceType DeviceType, mnemonic string) (wire.Message, error) {
	dev, err := getDevice(deviceType)
	if err != nil {
		return wire.Message{}, err
	}
	defer dev.Close()

	// Send SetMnemonic

	skycoinSetMnemonic := &messages.SetMnemonic{
		Mnemonic: proto.String(mnemonic),
	}

	data, err := proto.Marshal(skycoinSetMnemonic)
	if err != nil {
		return wire.Message{}, err
	}

	chunks := makeTrezorMessage(data, messages.MessageType_MessageType_SetMnemonic)

	msg, err := sendToDevice(dev, chunks)
	if err != nil {
		return wire.Message{}, err
	}

	if msg.Kind == uint16(messages.MessageType_MessageType_ButtonRequest) {
		msg, err = deviceButtonAck(dev)
		if err != nil {
			return wire.Message{}, err
		}
	}

	return msg, err
}

// SignMessage Ask the device to sign a message using the secret key at given index.
func (d *SkyWallet) SignMessage(deviceType DeviceType, addressN int, message string) (wire.Message, error) {
	dev, err := getDevice(deviceType)
	if err != nil {
		return wire.Message{}, err
	}
	defer dev.Close()

	skycoinSignMessage := &messages.SkycoinSignMessage{
		AddressN: proto.Uint32(uint32(addressN)),
		Message:  proto.String(message),
	}

	data, err := proto.Marshal(skycoinSignMessage)
	if err != nil {
		return wire.Message{}, err
	}

	chunks := makeTrezorMessage(data, messages.MessageType_MessageType_SkycoinSignMessage)

	return sendToDevice(dev, chunks)
}

// TransactionSign Ask the device to sign a transaction using the given information.
func (d *SkyWallet) TransactionSign(deviceType DeviceType, inputs []*messages.SkycoinTransactionInput, outputs []*messages.SkycoinTransactionOutput) (wire.Message, error) {
	dev, err := getDevice(deviceType)
	if err != nil {
		return wire.Message{}, err
	}
	defer dev.Close()

	skycoinTransactionSignMessage := &messages.TransactionSign{
		NbIn:           proto.Uint32(uint32(len(inputs))),
		NbOut:          proto.Uint32(uint32(len(outputs))),
		TransactionIn:  inputs,
		TransactionOut: outputs,
	}
	log.Println(skycoinTransactionSignMessage)

	data, err := proto.Marshal(skycoinTransactionSignMessage)
	if err != nil {
		return wire.Message{}, err
	}

	chunks := makeTrezorMessage(data, messages.MessageType_MessageType_TransactionSign)

	return sendToDevice(dev, chunks)
}

// WipeDevice wipes out device configuration
func (d *SkyWallet) Wipe(deviceType DeviceType) (wire.Message, error) {
	dev, err := getDevice(deviceType)
	if err != nil {
		return wire.Message{}, err
	}

	defer dev.Close()
	var chunks [][64]byte

	err = initialize(dev)
	if err != nil {
		return wire.Message{}, err
	}

	wipeDevice := &messages.WipeDevice{}
	data, err := proto.Marshal(wipeDevice)
	if err != nil {
		return wire.Message{}, err
	}

	var msg wire.Message
	chunks = makeTrezorMessage(data, messages.MessageType_MessageType_WipeDevice)
	msg, err = sendToDevice(dev, chunks)
	if err != nil {
		return wire.Message{}, err
	}
	log.Printf("Wipe device %d! Answer is: %x\n", msg.Kind, msg.Data)

	if msg.Kind == uint16(messages.MessageType_MessageType_ButtonRequest) {
		msg, err = deviceButtonAck(dev)
		if err != nil {
			return wire.Message{}, err
		}
	}

	if msg.Kind == uint16(messages.MessageType_MessageType_ButtonRequest) {
		err = initialize(dev)
		if err != nil {
			return wire.Message{}, err
		}
	}

	return msg, err
}

// ButtonAck when the device is waiting for the user to press a button
// the PC need to acknowledge, showing it knows we are waiting for a user action
func (s *SkyWallet) ButtonAck(deviceType DeviceType) (wire.Message, error) {
	dev, err := getDevice(deviceType)
	if err != nil {
		return wire.Message{}, err
	}
	defer dev.Close()
	return deviceButtonAck(dev)
}

func deviceButtonAck(dev io.ReadWriteCloser) (wire.Message, error) {
	var msg wire.Message
	// Send ButtonAck
	chunks, err := MessageButtonAck()
	if err != nil {
		return msg, err
	}
	err = sendToDeviceNoAnswer(dev, chunks)
	if err != nil {
		return msg, err
	}

	_, err = msg.ReadFrom(dev)
	time.Sleep(1 * time.Second)
	if err != nil {
		return msg, err
	}
	return msg, nil
}

// PassphraseAck send this message when the device is waiting for the user to input a passphrase
func (s *SkyWallet) PassphraseAck(deviceType DeviceType, passphrase string) (wire.Message, error) {
	dev, err := getDevice(deviceType)
	if err != nil {
		return wire.Message{}, err
	}
	defer dev.Close()

	chunks, err := MessagePassphraseAck(passphrase)
	if err != nil {
		return wire.Message{}, err
	}
	return sendToDevice(dev, chunks)
}

// WordAck send a word to the device during device "recovery procedure"
func (s *SkyWallet) WordAck(deviceType DeviceType, word string) (wire.Message, error) {
	dev, err := getDevice(deviceType)
	if err != nil {
		return wire.Message{}, err
	}

	defer dev.Close()
	chunks, err := MessageWordAck(word)
	if err != nil {
		return wire.Message{}, err
	}
	msg, err := sendToDevice(dev, chunks)
	if err != nil {
		return wire.Message{}, err
	}

	return msg, nil
}

// PinMatrixAck during PIN code setting use this message to send user input to device
func (s *SkyWallet) PinMatrixAck(deviceType DeviceType, p string) (wire.Message, error) {
	time.Sleep(1 * time.Second)
	dev, err := getDevice(deviceType)
	if err != nil {
		return wire.Message{}, err
	}
	defer dev.Close()

	log.Printf("Setting pin: %s\n", p)
	pinAck := &messages.PinMatrixAck{
		Pin: proto.String(p),
	}
	data, err := proto.Marshal(pinAck)
	if err != nil {
		return wire.Message{}, err
	}

	chunks := makeTrezorMessage(data, messages.MessageType_MessageType_PinMatrixAck)
	return sendToDevice(dev, chunks)
}
