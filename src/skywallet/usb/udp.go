package usb

import (
	"bytes"
	"io"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

var emulatorPing = []byte("PINGPING")
var emulatorPong = []byte("PONGPONG")

const (
	emulatorPrefix      = "emulator"
	emulatorAddress     = "127.0.0.1"
	emulatorPingTimeout = 5000 * time.Millisecond
)

type udpLowlevel struct {
	ping   chan []byte
	data   chan []byte
	writer io.Writer
}

type UDP struct {
	ports     []int
	lowlevels map[int]*udpLowlevel
}

func listen(conn io.Reader) (chan []byte, chan []byte) {
	ping := make(chan []byte, 1)
	data := make(chan []byte, 100)
	go func() {
		for {
			buffer := make([]byte, 64)
			_, err := conn.Read(buffer)
			if err == nil {
				first := buffer[0]
				if first == '?' {
					data <- buffer
				}
				if first == 'P' {
					copied := make([]byte, 8)
					copy(copied, buffer)
					ping <- copied
				}
			}
		}
	}()
	return ping, data
}

func (udp *UDP) makeLowlevel(port int) error {
	address := emulatorAddress + ":" + strconv.Itoa(port)

	connection, err := net.Dial("udp", address)
	if err != nil {
		return err
	}

	ping, data := listen(connection)
	udp.lowlevels[port] = &udpLowlevel{
		ping:   ping,
		data:   data,
		writer: connection,
	}
	return nil
}

func InitUDP(ports []int) (*UDP, error) {
	udp := UDP{
		ports:     ports,
		lowlevels: make(map[int](*udpLowlevel)),
	}
	for _, port := range ports {
		err := udp.makeLowlevel(port)
		if err != nil {
			return nil, err
		}
	}
	return &udp, nil
}

func checkPort(ping chan []byte, w io.Writer) (bool, error) {
	_, err := w.Write(emulatorPing)
	if err != nil {
		return false, err
	}
	select {
	case response := <-ping:
		return bytes.Equal(response, emulatorPong), nil
	case <-time.After(emulatorPingTimeout):
		return false, nil
	}
}

func (udp *UDP) Enumerate(_, _ uint16) ([]Info, error) {
	var infos []Info

	for _, port := range udp.ports {
		normal := udp.lowlevels[port]
		presentN, err := checkPort(normal.ping, normal.writer)
		if err != nil {
			return nil, err
		}
		if presentN {
			info := Info{
				Path:      emulatorPrefix + strconv.Itoa(port),
				VendorID:  0,
				ProductID: 0,
				Type:      TypeEmulator,
			}

			infos = append(infos, info)
		}
	}
	return infos, nil
}

func (udp *UDP) Has(path string) bool {
	return strings.HasPrefix(path, emulatorPrefix)
}

func (udp *UDP) Connect(path string) (Device, error) {
	port, err := strconv.Atoi(strings.TrimPrefix(path, emulatorPrefix))
	if err != nil {
		return nil, err
	}

	d := &UDPDevice{
		lowlevel: udp.lowlevels[port],
	}
	return d, nil
}

func (udp *UDP) Close() {
	// nothing
}

type UDPDevice struct {
	lowlevel *udpLowlevel

	closed int32 // atomic
}

func (d *UDPDevice) Close(disconnected bool) error {
	atomic.StoreInt32(&d.closed, 1)
	return nil
}

func (d *UDPDevice) readWrite(buf []byte, read bool) (int, error) {
	lowlevel := d.lowlevel
	for {
		closed := (atomic.LoadInt32(&d.closed)) == 1
		if closed {
			return 0, ErrClosedDevice
		}
		check, err := checkPort(lowlevel.ping, lowlevel.writer)
		if err != nil {
			return 0, err
		}
		if !check {
			return 0, ErrDisconnect
		}
		if !read {
			return lowlevel.writer.Write(buf)
		}

		select {
		case response := <-lowlevel.data:
			copy(buf, response)
			return len(response), nil
		case <-time.After(emulatorPingTimeout):
			// timeout, continue for cycle
		}
	}
}

func (d *UDPDevice) Write(buf []byte) (int, error) {
	return d.readWrite(buf, false)
}

func (d *UDPDevice) Read(buf []byte) (int, error) {
	return d.readWrite(buf, true)
}
