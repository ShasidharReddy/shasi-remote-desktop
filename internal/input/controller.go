package input

import (
	"fmt"
	"log"
	"runtime"
	"time"

	"github.com/ShasidharReddy/shasi-remote-desktop/internal/protocol"
)

type InputController struct {
	LastX      int
	LastY      int
	Throttle   time.Duration
	LastInput  time.Time
	OS         string
}

func NewInputController(throttleMs int) *InputController {
	return &InputController{
		Throttle:  time.Duration(throttleMs) * time.Millisecond,
		LastInput: time.Now(),
		OS:        runtime.GOOS,
	}
}

func (ic *InputController) ProcessInput(payload *protocol.InputPayload) error {
	// Throttle rapid inputs
	if time.Since(ic.LastInput) < ic.Throttle {
		return nil
	}
	ic.LastInput = time.Now()

	switch payload.Type {
	case "mouse_move":
		return ic.moveMouse(payload.X, payload.Y)
	case "mouse_click":
		return ic.clickMouse(payload.Button)
	case "key_press":
		return ic.pressKey(payload.KeyCode, payload.Key)
	case "key_release":
		return ic.releaseKey(payload.KeyCode, payload.Key)
	default:
		return fmt.Errorf("unknown input type: %s", payload.Type)
	}
}

func (ic *InputController) moveMouse(x, y int) error {
	ic.LastX, ic.LastY = x, y
	log.Printf("[Input] Mouse move: %d,%d (OS: %s)", x, y, ic.OS)
	// In production: Use platform-specific code to move mouse
	// macOS: CGEventCreateMouseEvent + CGEventPost
	// Windows: SetCursorPos()
	// Linux: xdotool mousemove
	return nil
}

func (ic *InputController) clickMouse(button int) error {
	var btn string
	switch button {
	case 1:
		btn = "left"
	case 2:
		btn = "middle"
	case 3:
		btn = "right"
	default:
		return fmt.Errorf("unknown button: %d", button)
	}

	log.Printf("[Input] Mouse click: %s button (OS: %s)", btn, ic.OS)
	// In production: Use platform-specific code to click mouse
	time.Sleep(50 * time.Millisecond)
	return nil
}

func (ic *InputController) pressKey(keyCode int, key string) error {
	log.Printf("[Input] Key press: %s (code: %d, OS: %s)", key, keyCode, ic.OS)
	// In production: Use platform-specific code to press key
	// macOS: CGEventCreateKeyboardEvent + CGEventPost
	// Windows: keybd_event()
	// Linux: xdotool key
	return nil
}

func (ic *InputController) releaseKey(keyCode int, key string) error {
	log.Printf("[Input] Key release: %s (code: %d, OS: %s)", key, keyCode, ic.OS)
	// In production: Use platform-specific code to release key
	return nil
}
