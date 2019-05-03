package integration

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	messages "github.com/skycoin/hardware-wallet-protob/go"
	"github.com/skycoin/skycoin/src/util/logging"
	"github.com/stretchr/testify/require"

	deviceWallet "github.com/skycoin/hardware-wallet-go/src/device-wallet"
)

var (
	log = logging.MustGetLogger("device-interface-tests")

	binaryPath string
)

const (
	binaryName = "hwgo-cli.test"

	testModeEmulator = "EMULATOR"
	testModeUSB      = "USB"
)

func execCommand(args ...string) *exec.Cmd {
	args = append(args)
	cmd := exec.Command(binaryPath, args...)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "AUTO_PRESS_BUTTONS=" + autopressButtons())
	return cmd
}

func execCommandCombinedOutput(args ...string) ([]byte, error) {
	cmd := execCommand(args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, err
	}
	return output, nil
}

func mode(t *testing.T) string {
	mode := os.Getenv("HW_GO_INTEGRATION_TEST_MODE")
	switch mode {
	case "":
		mode = testModeEmulator
	case testModeUSB, testModeEmulator:
	default:
		t.Fatalf("Invalid test mode %s, must be emulator or wallet", mode)
	}
	return mode
}

func enabled() bool {
	return os.Getenv("HW_GO_INTEGRATION_TESTS") == "1"
}

func autopressButtons() string {
	return os.Getenv("AUTO_PRESS_BUTTONS")
}

func TestMain(m *testing.M) {
	if !enabled() {
		return
	}

	err := os.Setenv("DEVICE_TYPE", os.Getenv("HW_GO_INTEGRATION_TEST_MODE"))
	if err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}

	abs, err := filepath.Abs(binaryName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "get binary name absolute path failed: %v\n", err)
		os.Exit(1)
	}

	binaryPath = abs

	// Build cli binary file.
	args := []string{"build", "-ldflags", "-X main.AUTO_PRESS_BUTTONS=true","-o", binaryPath, "../../../cmd/cli/cli.go"}
	if err := exec.Command("go", args...).Run(); err != nil {
		fmt.Fprintf(os.Stderr, fmt.Sprintf("Make %v binary failed: %v\n", binaryName, err))
		os.Exit(1)
	}

	ret := m.Run()

	// Remove the generated cli binary file.
	if err := os.Remove(binaryPath); err != nil {
		fmt.Fprintf(os.Stderr, "Delete %v failed: %v", binaryName, err)
		os.Exit(1)
	}

	os.Exit(ret)
}

func doWalletOrEmulator(t *testing.T) bool {
	if enabled() {
		switch mode(t) {
		case testModeUSB, testModeEmulator:
			return true
		}
	}

	t.Skip("usb and emulator tests disabled")
	return false
}

func bootstrap(t *testing.T, testname string) *deviceWallet.Device {
	if !doWalletOrEmulator(t) {
		return nil
	}

	device := deviceWallet.NewDevice(deviceWallet.DeviceTypeFromString(mode(t)))
	require.NotNil(t, device)

	err := device.Connect()
	require.NoError(t, err)

	if !device.Connected() {
		t.Skip(fmt.Sprintf("%s does not work if device is not connected", testname))
		return nil
	}
	require.NoError(t, device.Disconnect())

	if device.Driver.DeviceType() == deviceWallet.DeviceTypeEmulator && runtime.GOOS == "linux" {
		err := device.SetAutoPressButton(true, deviceWallet.ButtonRight)
		require.NoError(t, err)
	}

	// bootstrap

	// get features to check if bootstrap needs to be done
	msg, err := device.GetFeatures()
	require.NoError(t, err)
	require.Equal(t, msg.Kind, uint16(messages.MessageType_MessageType_Features))
	features := &messages.Features{}
	err = proto.Unmarshal(msg.Data, features)
	require.NoError(t, err)

	if *features.Initialized == false || *features.NeedsBackup == false {
		_, err = device.Wipe()
		require.NoError(t, err)
		_, err = device.SetMnemonic("cloud flower upset remain green metal below cup stem infant art thank")
		require.NoError(t, err)
	}

	return device
}

func TestAddressGen(t *testing.T) {
	device := bootstrap(t, "TestAddressGen")
	if device == nil {
		return
	}

	output, err := execCommandCombinedOutput([]string{"addressGen", "-addressN", "2"}...)
	if err != nil {
		require.EqualError(t, err, "exit status 1")
	}

	require.Contains(t, string(output), "[2EU3JbveHdkxW6z5tdhbbB2kRAWvXC2pLzw zC8GAQGQBfwk7vtTxVoRG7iMperHNuyYPs]")
}

func TestApplySettings(t *testing.T) {
	device := bootstrap(t, "TestApplySettings")
	if device == nil {
		return
	}

	var label = "my custom device label"
	output, err := execCommandCombinedOutput([]string{"applySettings", "-usePassphrase", "false", "-label", label}...)
	if err != nil {
		require.Equal(t, err, "exit status 1")
	}

	require.Contains(t, string(output), "Settings applied")

	output, err = execCommandCombinedOutput([]string{"features"}...)
	if err != nil {
		require.Equal(t, err, "exit status 1")
	}

	decoder := json.NewDecoder(bytes.NewReader(output))
	decoder.DisallowUnknownFields()

	var features messages.Features
	require.NoError(t, decoder.Decode(&features))
	require.Equal(t, *features.Label, label)
}

func TestBackup(t *testing.T) {
	device := bootstrap(t, "TestBackup")
	if device == nil {
		return
	}

	output, err := execCommandCombinedOutput([]string{"backup"}...)
	if err != nil {
		require.Equal(t, err, "exit status 1")
	}

	require.Contains(t, string(output), "Device backed up!")
}

func TestCheckMessageSignature(t *testing.T) {
	device := bootstrap(t, "TestCheckMessageSignature")
	if device == nil {
		return
	}

	output, err := execCommandCombinedOutput([]string{"checkMessageSignature",
		"--address", "2EU3JbveHdkxW6z5tdhbbB2kRAWvXC2pLzw", "--message", "Hello World", "--signature", "f026001ee2d4e6bf4dfdbbdd33b6622bae24c8f6232fc19e5dd0013f2b68f0f14606d448897df20348c8b63dcad6923e9fecb2fe0969b183dc31eb4796e001d101"}...)
	if err != nil {
		require.Equal(t, err, "exit status 1")
	}

	require.Contains(t, string(output), "2EU3JbveHdkxW6z5tdhbbB2kRAWvXC2pLzw")
}

func TestFeatures(t *testing.T) {
	device := bootstrap(t, "TestFeatures")
	if device == nil {
		return
	}

	output, err := execCommandCombinedOutput([]string{"features"}...)
	if err != nil {
		require.Equal(t, err, "exit status 1")
	}

	decoder := json.NewDecoder(bytes.NewReader(output))
	decoder.DisallowUnknownFields()

	var features messages.Features
	require.NoError(t, decoder.Decode(&features))
}

func TestGenerateMnemonic(t *testing.T) {
	if !doWalletOrEmulator(t) {
		return
	}

	device := deviceWallet.NewDevice(deviceWallet.DeviceTypeFromString(mode(t)))
	require.NotNil(t, device)

	err := device.Connect()
	require.NoError(t, err)

	if !device.Connected() {
		t.Skip(fmt.Sprintf("%s does not work if device is not connected", "TestGenerateMnemonic"))
		return
	}
	require.NoError(t, device.Disconnect())

	if device.Driver.DeviceType() == deviceWallet.DeviceTypeEmulator && runtime.GOOS == "linux" {
		err := device.SetAutoPressButton(true, deviceWallet.ButtonRight)
		require.NoError(t, err)
	}

	// bootstrap
	_, err = device.Wipe()
	require.NoError(t, err)

	output, err := execCommandCombinedOutput([]string{"generateMnemonic", "--wordCount", "12"}...)
	if err != nil {
		require.Equal(t, err, "exit status 1")
	}

	require.Contains(t, string(output), "Mnemonic successfully configured")
}

func TestRecovery(t *testing.T) {
	// This test checks for failure on invalid input
	// It is not possible to programmatically check for valid input

	if !doWalletOrEmulator(t) {
		return
	}

	device := deviceWallet.NewDevice(deviceWallet.DeviceTypeFromString(mode(t)))
	require.NotNil(t, device)

	err := device.Connect()
	require.NoError(t, err)

	if !device.Connected() {
		t.Skip(fmt.Sprintf("%s does not work if device is not connected", "TestRecovery"))
		return
	}
	require.NoError(t, device.Disconnect())

	if device.Driver.DeviceType() == deviceWallet.DeviceTypeEmulator && runtime.GOOS == "linux" {
		err := device.SetAutoPressButton(true, deviceWallet.ButtonRight)
		require.NoError(t, err)
	}

	// bootstrap
	_, err = device.Wipe()
	require.NoError(t, err)

	cmd := execCommand([]string{"recovery"}...)

	stdoutPipe, err := cmd.StdoutPipe()
	require.NoError(t, err)
	stderrPipe, err := cmd.StderrPipe()
	require.NoError(t, err)
	stdInPipe, err := cmd.StdinPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())

	var fail = false
	var stdInDone = false

	go func() {
		scanner := bufio.NewScanner(stdoutPipe)

		scanner.Split(bufio.ScanWords)
		for scanner.Scan() {
			m := scanner.Text()
			if m == "Word:" {
				time.Sleep(1 * time.Second)
				_, err := stdInPipe.Write([]byte("foobar\n"))
				require.NoError(t, err)

				stdInDone = true
			} else if stdInDone {
				if m == "Wrong" || m == "Word" {
					fail = true
					break
				}
			}
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		scanner.Split(bufio.ScanWords)
		for scanner.Scan() {
			log.Errorln(scanner.Text())
		}
	}()

	err = cmd.Wait()
	require.NoError(t, err)
	require.True(t, fail)
}

func TestSetMnemonic(t *testing.T) {
	if !doWalletOrEmulator(t) {
		return
	}

	device := deviceWallet.NewDevice(deviceWallet.DeviceTypeFromString(mode(t)))
	require.NotNil(t, device)

	err := device.Connect()
	require.NoError(t, err)

	if !device.Connected() {
		t.Skip(fmt.Sprintf("%s does not work if device is not connected", "TestGenerateMnemonic"))
		return
	}
	require.NoError(t, device.Disconnect())

	if device.Driver.DeviceType() == deviceWallet.DeviceTypeEmulator && runtime.GOOS == "linux" {
		err := device.SetAutoPressButton(true, deviceWallet.ButtonRight)
		require.NoError(t, err)
	}

	// bootstrap
	_, err = device.Wipe()
	require.NoError(t, err)

	mnemonic := "cloud flower upset remain green metal below cup stem infant art thank"
	output, err := execCommandCombinedOutput([]string{"setMnemonic", "--mnemonic", mnemonic}...)
	if err != nil {
		require.Equal(t, err, "exit status 1")
	}

	require.Contains(t, string(output), mnemonic)
}

func TestSetPinCode(t *testing.T) {
	// This test checks for failure on invalid input
	// It is not possible to programmatically check for valid input

	device := bootstrap(t, "TestSetPinCode")
	if device == nil {
		return
	}

	cmd := execCommand([]string{"setPinCode"}...)

	stdoutPipe, err := cmd.StdoutPipe()
	require.NoError(t, err)
	stderrPipe, err := cmd.StderrPipe()
	require.NoError(t, err)
	stdInPipe, err := cmd.StdinPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())

	var fail = false
	var stdInDone = false

	go func() {
		scanner := bufio.NewScanner(stdoutPipe)

		scanner.Split(bufio.ScanWords)
		for scanner.Scan() {
			m := scanner.Text()
			if m == "response:" {
				time.Sleep(1 * time.Second)
				_, err := stdInPipe.Write([]byte("123\n"))
				require.NoError(t, err)

				stdInDone = true
			} else if stdInDone {
				if m == "mismatch" {
					fail = true
					break
				}
			}
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		scanner.Split(bufio.ScanWords)
		for scanner.Scan() {
			log.Errorln(scanner.Text())
		}
	}()

	err = cmd.Wait()
	require.NoError(t, err)
	require.True(t, fail)
}

func TestRemovePinCode(t *testing.T) {
	device := bootstrap(t, "TestRemovePinCode")
	if device == nil {
		return
	}

	output, err := execCommandCombinedOutput([]string{"removePinCode"}...)
	if err != nil {
		require.Equal(t, err, "exit status 1")
	}

	require.Contains(t, string(output), "PIN removed")
}

func TestSignMessage(t *testing.T) {
	device := bootstrap(t, "TestTransactionSign")
	if device == nil {
		return
	}

	output, err := execCommandCombinedOutput([]string{"signMessage", "--message", "Hello World"}...)
	if err != nil {
		require.Equal(t, err, "exit status 1")
	}

	outputStr := strings.TrimSpace(string(bytes.Replace(output, []byte("PASS"), []byte{}, 1)))

	checkOutput, err := execCommandCombinedOutput([]string{"checkMessageSignature", "--address", "2EU3JbveHdkxW6z5tdhbbB2kRAWvXC2pLzw", "--message", "Hello World", "--signature", outputStr}...)
	if err != nil {
		require.Equal(t, err, "exit status 1")
	}

	require.Contains(t, string(checkOutput), "2EU3JbveHdkxW6z5tdhbbB2kRAWvXC2pLzw")
}

func TestTransactionSign(t *testing.T) {
	device := bootstrap(t, "TestTransactionSign")
	if device == nil {
		return
	}

	output, err := execCommandCombinedOutput([]string{"transactionSign", "--inputHash", "a885343cc57aedaab56ad88d860f2bd436289b0248d1adc55bcfa0d9b9b807c3", "--inputIndex", "0", "--outputAddress=zC8GAQGQBfwk7vtTxVoRG7iMperHNuyYPs", "--coin", "1000000", "--hour", "1"}...)
	if err != nil {
		require.Equal(t, err, "exit status 1")
	}

	require.Contains(t, string(output), "a885343cc57aedaab56ad88d860f2bd436289b0248d1adc55bcfa0d9b9b807c3")
}
