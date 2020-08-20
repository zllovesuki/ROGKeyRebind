package keyboard

/*
We are looking for a device that looks something like this:
\\?\hid#vid_0b05&pid_1866&mi_02&col01#8&1e16c781&0&0000#{4d1e55b2-f16f-11cf-88cb-001111000030}
Everything after \hid is just plain old PnP DeviceID, but with "\" replaced with "#"
"vid_X" where X is the vendor ID
"pid_X" where X is the product ID
"mi_X" and "colY" indicates that this is a multi-function, multi-TLC device, and we are looking for a specific column
&1e16c... is the serial number and it *should* be different on each computer
the {uuid} part is generic GUID_DEVINTERFACE_HID: https://docs.microsoft.com/en-us/windows-hardware/drivers/install/guid-devinterface-hid
*/

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/zllovesuki/ROGManager/system/device"
	"github.com/zllovesuki/ROGManager/system/persist"

	"github.com/karalabe/usb"
)

const (
	persistKey = "KeyboardControl"
)

const (
	writeControlCode = uint32(721297)
)

const (
	brightnessControlByteIndex = 4
)

const (
	brightnessControlBufferLength     = 64
	touchPadToggleControlBufferLength = 64
)

const (
	kbBrightnessDevice = "mi_02&col01"
)

var (
	brightnessControlBuffer = []byte{
		0x5a, 0xba, 0xc5, 0xc4, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
	touchPadToggleControlBuffer = []byte{
		0x5a, 0xf4, 0x6b, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
)

// Level defines the different level of keybroad brightness
type Level byte

// Brightness level
const (
	OFF    Level = 0x00
	LOW          = 0x01
	MEDIUM       = 0x02
	HIGH         = 0x03
)

// Control allows you to set the hid related functionalities directly
type Control struct {
	deviceCtrl        *device.Control
	currentBrightness Level
}

// NewControl checks if the computer has the hid control interface, and returns a control interface if it does
func NewControl() (*Control, error) {
	devices, err := usb.EnumerateHid(VendorID, ProductID)
	if err != nil {
		return nil, err
	}
	var path string
	for _, device := range devices {
		if strings.Contains(device.Path, kbBrightnessDevice) {
			path = device.Path
		}
	}
	if path == "" {
		return nil, fmt.Errorf("Keyboard control interface not found")
	}
	// I could technically use usb.Device.Write() here
	ctrl, err := device.NewControl(path, writeControlCode)
	if err != nil {
		return nil, err
	}
	return &Control{
		deviceCtrl:        ctrl,
		currentBrightness: OFF,
	}, nil
}

func (c *Control) setBrightness(v Level) error {
	inputBuf := make([]byte, brightnessControlBufferLength)
	copy(inputBuf, brightnessControlBuffer)
	inputBuf[brightnessControlByteIndex] = byte(v)

	_, err := c.deviceCtrl.Write(inputBuf)
	if err != nil {
		return err
	}

	c.currentBrightness = v

	return nil
}

// BrightnessUp increases the keyboard brightness by one level
// TODO: use a FSM
func (c *Control) BrightnessUp() error {
	var targetLevel Level
	switch c.currentBrightness {
	case OFF:
		targetLevel = LOW
	case LOW:
		targetLevel = MEDIUM
	case MEDIUM:
		targetLevel = HIGH
	default:
		return nil
	}
	return c.setBrightness(targetLevel)
}

// BrightnessDown decreases the keyboard brightness by one level
// TODO: use a FSM
func (c *Control) BrightnessDown() error {
	var targetLevel Level
	switch c.currentBrightness {
	case HIGH:
		targetLevel = MEDIUM
	case MEDIUM:
		targetLevel = LOW
	case LOW:
		targetLevel = OFF
	default:
		return nil
	}
	return c.setBrightness(targetLevel)
}

// ToggleTouchPad will toggle between disabling/enabling TouchPad
func (c *Control) ToggleTouchPad() error {
	inputBuf := make([]byte, touchPadToggleControlBufferLength)
	copy(inputBuf, touchPadToggleControlBuffer)

	_, err := c.deviceCtrl.Write(inputBuf)
	if err != nil {
		return err
	}

	// I don't think we have a way of checking if the touchpad is disabled/enabled

	return nil
}

var _ persist.Registry = &Control{}

// Name satisfies persist.Registry
func (c *Control) Name() string {
	return persistKey
}

// Value satisfies persist.Registry
func (c *Control) Value() []byte {
	buf := make([]byte, 2)
	binary.LittleEndian.PutUint16(buf, uint16(c.currentBrightness))
	return buf
}

// Load satisfies persist.Registry
// TODO: check if the input is actually valid
func (c *Control) Load(v []byte) error {
	if len(v) == 0 {
		return nil
	}
	c.currentBrightness = Level(binary.LittleEndian.Uint16(v))
	return nil
}

// Apply satisfies persist.Registry
func (c *Control) Apply() error {
	return c.setBrightness(c.currentBrightness)
}

// Close satisfied persist.Registry
func (c *Control) Close() error {
	return c.deviceCtrl.Close()
}