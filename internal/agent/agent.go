package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ShasidharReddy/shasi-remote-desktop/internal/files"
	"github.com/ShasidharReddy/shasi-remote-desktop/internal/input"
	"github.com/ShasidharReddy/shasi-remote-desktop/internal/protocol"
	"github.com/ShasidharReddy/shasi-remote-desktop/internal/screen"
)

type Agent struct {
	AgentID         string
	ServerAddr      string
	Conn            *websocket.Conn
	ScreenCapture   *screen.ScreenCapture
	InputController *input.InputController
	FileManager     *files.FileTransferManager
	FrameTicker     *time.Ticker
	QuitChan        chan struct{}
}

func NewAgent(agentID, serverAddr string) *Agent {
	homeDir, _ := os.UserHomeDir()
	uploadDir := filepath.Join(homeDir, ".shasi-remote", "uploads")
	os.MkdirAll(uploadDir, 0755)

	return &Agent{
		AgentID:         agentID,
		ServerAddr:      serverAddr,
		ScreenCapture:   screen.NewScreenCapture(15, 80),
		InputController: input.NewInputController(50),
		FileManager:     files.NewFileTransferManager(uploadDir),
		QuitChan:        make(chan struct{}),
	}
}

func (a *Agent) Connect() error {
	u := url.URL{Scheme: "ws", Host: a.ServerAddr, Path: "/ws"}
	log.Printf("Agent connecting to %s", u.String())

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("dial error: %w", err)
	}
	a.Conn = conn
	defer conn.Close()

	// Register as agent
	regMsg := protocol.Message{
		Type: protocol.TypeRegister,
		Payload: toRawMessage(&protocol.RegisterPayload{
			AgentID: a.AgentID,
			Role:    "agent",
		}),
	}
	if err := conn.WriteJSON(regMsg); err != nil {
		return fmt.Errorf("register error: %w", err)
	}

	log.Printf("Agent registered: %s", a.AgentID)

	// Start screen capture loop
	go a.captureLoop()

	// Handle incoming messages
	a.handleMessages(conn)

	return nil
}

func (a *Agent) captureLoop() {
	a.FrameTicker = time.NewTicker(time.Second / time.Duration(a.ScreenCapture.FPS))
	defer a.FrameTicker.Stop()

	for {
		select {
		case <-a.QuitChan:
			return
		case <-a.FrameTicker.C:
			frame, err := a.ScreenCapture.CaptureFrame()
			if err != nil {
				log.Printf("Capture error: %v", err)
				continue
			}

			msg := protocol.Message{
				Type:    protocol.TypeScreenFrame,
				AgentID: a.AgentID,
				Payload: toRawMessage(frame),
			}

			if err := a.Conn.WriteJSON(msg); err != nil {
				log.Printf("Send frame error: %v", err)
				return
			}
		}
	}
}

func (a *Agent) handleMessages(conn *websocket.Conn) {
	for {
		var msg protocol.Message
		if err := conn.ReadJSON(&msg); err != nil {
			log.Printf("Read error: %v", err)
			return
		}

		switch msg.Type {
		case protocol.TypeInput:
			var payload protocol.InputPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				log.Printf("Unmarshal error: %v", err)
				continue
			}
			if err := a.InputController.ProcessInput(&payload); err != nil {
				log.Printf("Input error: %v", err)
			}

		case protocol.TypeFileTransfer:
			var payload protocol.FileTransferPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				log.Printf("Unmarshal error: %v", err)
				continue
			}
			if err := a.FileManager.StartTransfer(&payload); err != nil {
				log.Printf("Transfer error: %v", err)
			}

		case protocol.TypeFileChunk:
			var payload protocol.FileChunkPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				log.Printf("Unmarshal error: %v", err)
				continue
			}
			if err := a.FileManager.ReceiveChunk(&payload); err != nil {
				log.Printf("Chunk error: %v", err)
			}

		case protocol.TypeFileEnd:
			var payload protocol.FileEndPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				log.Printf("Unmarshal error: %v", err)
				continue
			}
			if err := a.FileManager.EndTransfer(&payload); err != nil {
				log.Printf("End transfer error: %v", err)
			}

		case protocol.TypePing:
			pongMsg := protocol.Message{Type: protocol.TypePong, AgentID: a.AgentID}
			conn.WriteJSON(pongMsg)
		}
	}
}

func toRawMessage(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}
