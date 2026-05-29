package viewer

import (
	"encoding/json"
	"fmt"
	"image"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ShasidharReddy/shasi-remote-desktop/internal/protocol"
	"github.com/ShasidharReddy/shasi-remote-desktop/internal/screen"
	"github.com/gorilla/websocket"
)

type Viewer struct {
	AgentID      string
	ServerAddr   string
	Conn         *websocket.Conn
	ScreenCache  *screen.ScreenCapture
	CurrentFrame image.Image
	LastFrame    time.Time
	QuitChan     chan struct{}
	writeMu      sync.Mutex
	closeOnce    sync.Once
}

func NewViewer(agentID, serverAddr string) *Viewer {
	return &Viewer{
		AgentID:     agentID,
		ServerAddr:  serverAddr,
		ScreenCache: screen.NewScreenCapture(15, 80),
		QuitChan:    make(chan struct{}),
	}
}

func (v *Viewer) Connect() error {
	u := url.URL{Scheme: "ws", Host: v.ServerAddr, Path: "/ws"}
	log.Printf("Viewer connecting to %s for agent %s", u.String(), v.AgentID)

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("dial error: %w", err)
	}
	v.Conn = conn
	defer conn.Close()
	defer v.closeQuitChan()

	regMsg := protocol.Message{
		Type: protocol.TypeRegister,
		Payload: toRawMessage(&protocol.RegisterPayload{
			AgentID: v.AgentID,
			Role:    "viewer",
		}),
	}
	if err := v.writeJSON(regMsg); err != nil {
		return fmt.Errorf("register error: %w", err)
	}

	log.Printf("Viewer registered for agent: %s", v.AgentID)

	go v.keepalive()
	go v.controlLoop()
	v.handleMessages()

	return nil
}

func (v *Viewer) handleMessages() {
	for {
		var msg protocol.Message
		if err := v.Conn.ReadJSON(&msg); err != nil {
			log.Printf("Read error: %v", err)
			return
		}

		switch msg.Type {
		case protocol.TypeScreenFrame:
			var payload protocol.ScreenFramePayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				log.Printf("Unmarshal error: %v", err)
				continue
			}

			img, err := v.ScreenCache.DecodeFrame(payload.Data)
			if err != nil {
				log.Printf("Decode error: %v", err)
				continue
			}

			v.CurrentFrame = img
			v.LastFrame = time.Now()
			log.Printf("Frame received: %dx%d", payload.Width, payload.Height)

		case protocol.TypePong:
			log.Printf("Pong received")
		}
	}
}

func (v *Viewer) keepalive() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-v.QuitChan:
			return
		case <-ticker.C:
			pingMsg := protocol.Message{
				Type:    protocol.TypePing,
				AgentID: v.AgentID,
			}
			if err := v.writeJSON(pingMsg); err != nil {
				log.Printf("Ping error: %v", err)
				return
			}
		}
	}
}

func (v *Viewer) controlLoop() {
	log.Println("Viewer ready. Press 'h' for help, 'q' to quit")
	log.Println("Send input commands via stdin (format: 'move:x:y', 'click:1', 'key:enter')")

	var cmd string
	for {
		fmt.Print("> ")
		if _, err := fmt.Scanln(&cmd); err != nil {
			log.Printf("Input error: %v", err)
			v.closeQuitChan()
			if v.Conn != nil {
				v.Conn.Close()
			}
			return
		}

		if cmd == "q" {
			v.closeQuitChan()
			if v.Conn != nil {
				v.Conn.Close()
			}
			return
		}
		if cmd == "h" {
			v.printHelp()
			continue
		}

		parts := strings.SplitN(cmd, ":", 3)
		var payload protocol.InputPayload

		switch parts[0] {
		case "move":
			if len(parts) != 3 {
				fmt.Printf("Invalid move command: %s\n", cmd)
				continue
			}
			x, err := strconv.Atoi(parts[1])
			if err != nil {
				fmt.Printf("Invalid X coordinate: %s\n", parts[1])
				continue
			}
			y, err := strconv.Atoi(parts[2])
			if err != nil {
				fmt.Printf("Invalid Y coordinate: %s\n", parts[2])
				continue
			}
			payload = protocol.InputPayload{Type: "mouse_move", X: x, Y: y}
		case "click":
			if len(parts) != 2 {
				fmt.Printf("Invalid click command: %s\n", cmd)
				continue
			}
			button, err := strconv.Atoi(parts[1])
			if err != nil {
				fmt.Printf("Invalid button: %s\n", parts[1])
				continue
			}
			payload = protocol.InputPayload{Type: "mouse_click", Button: button}
		case "key":
			if len(parts) != 2 {
				fmt.Printf("Invalid key command: %s\n", cmd)
				continue
			}
			payload = protocol.InputPayload{Type: "key_press", Key: parts[1]}
		default:
			fmt.Printf("Unknown command: %s\n", cmd)
			continue
		}

		inputMsg := protocol.Message{
			Type:    protocol.TypeInput,
			AgentID: v.AgentID,
			Payload: toRawMessage(&payload),
		}

		log.Printf("Command: %s", cmd)
		if err := v.writeJSON(inputMsg); err != nil {
			log.Printf("Send error: %v", err)
		}
	}
}

func (v *Viewer) SendInput(inputPayload *protocol.InputPayload) error {
	msg := protocol.Message{
		Type:    protocol.TypeInput,
		AgentID: v.AgentID,
		Payload: toRawMessage(inputPayload),
	}
	return v.writeJSON(msg)
}

func (v *Viewer) SendFileTransfer(filePath string) error {
	stat, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("stat error: %w", err)
	}

	msg := protocol.Message{
		Type:    protocol.TypeFileTransfer,
		AgentID: v.AgentID,
		Payload: toRawMessage(&protocol.FileTransferPayload{
			FileName: filepath.Base(filePath),
			FileSize: stat.Size(),
			FileID:   fmt.Sprintf("%s_%d", filepath.Base(filePath), time.Now().UnixNano()),
		}),
	}
	return v.writeJSON(msg)
}

func (v *Viewer) writeJSON(msg interface{}) error {
	v.writeMu.Lock()
	defer v.writeMu.Unlock()
	return v.Conn.WriteJSON(msg)
}

func (v *Viewer) closeQuitChan() {
	if v.QuitChan == nil {
		return
	}
	v.closeOnce.Do(func() {
		close(v.QuitChan)
	})
}

func (v *Viewer) printHelp() {
	fmt.Println("Commands:")
	fmt.Println("  h - show help")
	fmt.Println("  q - quit")
	fmt.Println("  move:X:Y - move mouse to coordinates")
	fmt.Println("  click:BUTTON - click mouse (1=left, 2=middle, 3=right)")
	fmt.Println("  key:KEYNAME - press key (enter, space, escape, etc)")
}

func toRawMessage(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}
