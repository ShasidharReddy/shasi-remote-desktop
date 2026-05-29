package server

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/ShasidharReddy/shasi-remote-desktop/internal/protocol"
	"github.com/gorilla/websocket"
)

type Client struct {
	AgentID string
	Role    string
	Conn    *websocket.Conn
	Send    chan interface{}
	Peer    *Client
	mu      sync.Mutex
}

type RelayServer struct {
	addr    string
	clients map[string]*Client
	mu      sync.RWMutex
	upgrade websocket.Upgrader
}

func NewRelayServer(addr string) *RelayServer {
	return &RelayServer{
		addr:    addr,
		clients: make(map[string]*Client),
		upgrade: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (s *RelayServer) Start() error {
	http.HandleFunc("/ws", s.handleWS)
	http.HandleFunc("/status", s.handleStatus)
	log.Printf("Relay server listening on %s", s.addr)
	return http.ListenAndServe(s.addr, nil)
}

func (s *RelayServer) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrade.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}

	client := &Client{
		Conn: conn,
		Send: make(chan interface{}, 100),
	}

	var msg protocol.Message
	if err := conn.ReadJSON(&msg); err != nil {
		log.Printf("Read error: %v", err)
		conn.Close()
		return
	}

	if msg.Type != protocol.TypeRegister {
		log.Printf("Expected register, got %s", msg.Type)
		conn.Close()
		return
	}

	var regPayload protocol.RegisterPayload
	if err := json.Unmarshal(msg.Payload, &regPayload); err != nil {
		log.Printf("Unmarshal error: %v", err)
		conn.Close()
		return
	}

	client.AgentID = regPayload.AgentID
	client.Role = regPayload.Role

	s.mu.Lock()
	key := regPayload.AgentID + ":" + regPayload.Role
	existing, exists := s.clients[key]
	if exists && existing.Conn != nil {
		existing.Conn.Close()
	}
	s.clients[key] = client
	s.mu.Unlock()

	log.Printf("Client registered: %s (%s)", client.AgentID, client.Role)

	go s.writePump(client)
	s.readPump(client)
}

func (s *RelayServer) readPump(client *Client) {
	defer func() {
		s.mu.Lock()
		key := client.AgentID + ":" + client.Role
		delete(s.clients, key)
		s.mu.Unlock()
		close(client.Send)
		client.Conn.Close()
		log.Printf("Client disconnected: %s (%s)", client.AgentID, client.Role)
	}()

	for {
		var msg protocol.Message
		if err := client.Conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
				log.Printf("WebSocket error: %v", err)
			}
			return
		}

		msg.AgentID = client.AgentID

		s.mu.RLock()
		peerRole := "agent"
		if client.Role == "agent" {
			peerRole = "viewer"
		}
		peerKey := client.AgentID + ":" + peerRole
		peer, exists := s.clients[peerKey]
		s.mu.RUnlock()

		if exists && peer != nil {
			if !safeSend(peer.Send, msg) {
				log.Printf("Peer send failed: %s", peerKey)
			}
		}
	}
}

func (s *RelayServer) writePump(client *Client) {
	for msg := range client.Send {
		if err := client.Conn.WriteJSON(msg); err != nil {
			log.Printf("Write error: %v", err)
			return
		}
	}
}

func safeSend(ch chan interface{}, msg interface{}) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()

	select {
	case ch <- msg:
		return true
	default:
		return false
	}
}

func (s *RelayServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := map[string]interface{}{
		"connected_clients": len(s.clients),
		"clients":           make([]map[string]string, 0),
	}

	for key, client := range s.clients {
		status["clients"] = append(status["clients"].([]map[string]string), map[string]string{
			"agent_id": client.AgentID,
			"role":     client.Role,
			"key":      key,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
