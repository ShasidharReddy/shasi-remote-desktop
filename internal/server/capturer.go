package server

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// ─── capturer ─────────────────────────────────────────────────────────────────

type capturer struct {
	fileMu sync.Mutex
	files  map[string]*inFile
}

type inFile struct {
	name string
	path string
	f    *os.File
}

func newCapturer() *capturer {
	return &capturer{files: make(map[string]*inFile)}
}

// capture takes a screenshot and returns base64-encoded JPEG + dimensions.
func (c *capturer) capture() (b64 string, w, h int, err error) {
	var data []byte

	switch runtime.GOOS {
	case "darwin":
		data, w, h, err = captureMacOS()
	case "linux":
		data, w, h, err = captureLinux()
	default:
		data, w, h, err = capturePlaceholder()
	}
	if err != nil {
		return "", 0, 0, err
	}
	return base64.StdEncoding.EncodeToString(data), w, h, nil
}

func captureMacOS() ([]byte, int, int, error) {
	tmp := fmt.Sprintf("/tmp/shasi_%d.jpg", time.Now().UnixNano())
	if err := exec.Command("screencapture", "-x", "-t", "jpg", tmp).Run(); err != nil {
		return capturePlaceholder()
	}
	defer os.Remove(tmp)

	data, err := os.ReadFile(tmp)
	if err != nil {
		return capturePlaceholder()
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return data, 1920, 1080, nil
	}
	b := img.Bounds()
	return data, b.Dx(), b.Dy(), nil
}

func captureLinux() ([]byte, int, int, error) {
	tmp := fmt.Sprintf("/tmp/shasi_%d.jpg", time.Now().UnixNano())
	if err := exec.Command("scrot", "-q", "70", tmp).Run(); err != nil {
		// fallback: ImageMagick
		if err2 := exec.Command("import", "-window", "root", "-quality", "70", tmp).Run(); err2 != nil {
			return capturePlaceholder()
		}
	}
	defer os.Remove(tmp)
	data, err := os.ReadFile(tmp)
	if err != nil {
		return capturePlaceholder()
	}
	return data, 1920, 1080, nil
}

func capturePlaceholder() ([]byte, int, int, error) {
	img := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	for y := 0; y < 720; y++ {
		for x := 0; x < 1280; x++ {
			img.Set(x, y, color.RGBA{26, 26, 46, 255})
		}
	}
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, &jpeg.Options{Quality: 70})
	return buf.Bytes(), 1280, 720, nil
}

// ─── input injection ─────────────────────────────────────────────────────────

func (c *capturer) executeInput(msg WsMsg) {
	switch runtime.GOOS {
	case "darwin":
		executeInputMacOS(msg)
	case "linux":
		executeInputLinux(msg)
	}
}

func executeInputMacOS(msg WsMsg) {
	var script string
	switch msg.IType {
	case "mouse_move":
		script = fmt.Sprintf(
			`tell application "System Events" to set position of mouse to {%d, %d}`,
			msg.X, msg.Y)
	case "mouse_click":
		btn := "button 1"
		if msg.Button == 3 {
			btn = "button 2"
		}
		script = fmt.Sprintf(
			`tell application "System Events" to click at {%d, %d} using %s`,
			msg.X, msg.Y, btn)
	case "key_press":
		// escape special chars
		k := msg.Key
		if len(k) == 1 {
			script = fmt.Sprintf(
				`tell application "System Events" to keystroke "%s"`, k)
		} else {
			// named keys: return, space, escape, tab, etc.
			script = fmt.Sprintf(
				`tell application "System Events" to key code %s`,
				appleKeyCode(k))
		}
	default:
		return
	}
	exec.Command("osascript", "-e", script).Run()
}

func appleKeyCode(key string) string {
	m := map[string]string{
		"Enter": "36", "Return": "36",
		"Backspace": "51", "Delete": "51",
		"Escape": "53",
		"Tab": "48",
		"ArrowLeft": "123", "ArrowRight": "124",
		"ArrowDown": "125", "ArrowUp": "126",
		"Space": "49",
	}
	if code, ok := m[key]; ok {
		return code
	}
	return "36" // fallback: return
}

func executeInputLinux(msg WsMsg) {
	switch msg.IType {
	case "mouse_move":
		exec.Command("xdotool", "mousemove", fmt.Sprint(msg.X), fmt.Sprint(msg.Y)).Run()
	case "mouse_click":
		exec.Command("xdotool", "click", "1").Run()
	case "key_press":
		exec.Command("xdotool", "key", msg.Key).Run()
	}
}

// ─── file transfer ────────────────────────────────────────────────────────────

func (c *capturer) startFile(fileID, fileName string, fileSize int64, uploadDir string) {
	c.fileMu.Lock()
	defer c.fileMu.Unlock()

	safe := filepath.Base(fileName)
	path := filepath.Join(uploadDir, safe)
	f, err := os.Create(path)
	if err != nil {
		log.Printf("Create file error: %v", err)
		return
	}
	c.files[fileID] = &inFile{name: fileName, path: path, f: f}
	log.Printf("File incoming: %s -> %s (%.2f MB)", fileID, safe, float64(fileSize)/1e6)
}

func (c *capturer) writeChunk(fileID, b64data string) {
	c.fileMu.Lock()
	defer c.fileMu.Unlock()

	inf, ok := c.files[fileID]
	if !ok {
		return
	}
	data, err := base64.StdEncoding.DecodeString(b64data)
	if err != nil {
		log.Printf("Decode chunk error: %v", err)
		return
	}
	if _, err := inf.f.Write(data); err != nil {
		log.Printf("Write chunk error: %v", err)
	}
}

func (c *capturer) finishFile(fileID string) string {
	c.fileMu.Lock()
	defer c.fileMu.Unlock()

	inf, ok := c.files[fileID]
	if !ok {
		return ""
	}
	inf.f.Close()
	delete(c.files, fileID)
	log.Printf("File complete: %s", inf.path)
	return inf.path
}
