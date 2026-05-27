package viewer

import (
	"encoding/json"
	"fmt"
	"image"
	"log"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ShasidharReddy/shasi-remote-desktop/internal/protocol"
	"github.com/ShasidharReddy/shasi-remote-desktop/internal/screen"
)

type Viewer struct {
	AgentID      string
	ServerAddr   string
	Conn         *websocket.Conn
	ScreenCache  *screen.ScreenCapture
	CurrentFrame image.Image
	LastFrame    time.Time
	QuitChan     chan struct{}
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

	// Register as viewer
	regMsg := protocol.Message{
		Type: protocol.TypeRegister,
		Payload: toRawMessage(&protocol.RegisterPayload{
			AgentID: v.AgentID,
			Role:    "viewer",
		}),
	}
	if err := conn.WriteJSON(regMsg); err != nil {
		return fmt.Errorf("register error: %w", err)
	}

	log.Printf("Viewer registered for agent: %s", v.AgentID)

	// Start keepalive
	go v.keepalive(conn)

	// Start interactive control loop (simple CLI for now)
	go v.controlLoop(conn)

	// Handle incoming messages
	v.handleMessages(conn)

	return nil
}

func (v *Viewer) handleMessages(conn *websocket.Conn) {
	for {
		var msg protocol.Message
		if err := conn.ReadJSON(&msg); err != nil {
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

func (v *Viewer) keepalive(conn *websocket.Conn) {
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
			if err := conn.WriteJSON(pingMsg); err != nil {
				log.Printf("Ping error: %v", err)
				return
			}
		}
	}
}

func (v *Viewer) controlLoop(conn *websocket.Conn) {
	log.Println("Viewer ready. Press 'h' for help, 'q' to quit")
	log.Println("Send input commands via stdin (format: 'move:x:y', 'click:1', 'key:enter')")

	var cmd string
	for {
		fmt.Print("> ")
		fmt.Scanln(&cmd)

		if cmd == "q" {
			close(v.QuitChan)
			return
		}
		if cmd == "h" {
			v.printHelp()
			continue
		}

		// Parse simple command format
		// move:x:y - mouse move
		// click:button - mouse click (1,2,3)
		// key:keyname - keyboard key
		var inputMsg protocol.Message
		inputMsg.Type = protocol.TypeInput
		inputMsg.AgentID = v.AgentID

		// Simple parsing (production would use proper parsing)
		log.Printf("Command: %s", cmd)

		if err := conn.WriteJSON(inputMsg); err != nil {
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
	return v.Conn.WriteJSON(msg)
}

func (v *Viewer) SendFileTransfer(filePath string) error {
	msg := protocol.Message{
		Type:    protocol.TypeFileTransfer,
		AgentID: v.AgentID,
		Payload: toRawMessage(&protocol.FileTransferPayload{
			FileName: filePath,
			FileSize: 0,
			FileID:   fmt.Sprintf("file_%d", time.Now().UnixNano()),
		}),
	}
	return v.Conn.WriteJSON(msg)
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
