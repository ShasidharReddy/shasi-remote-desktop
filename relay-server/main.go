// Secure System — Cloud Relay Server
// Deployed once (Render/Railway free tier); both clients connect to it.
// Routes all traffic (signaling + screen frames) between host and viewer by machine ID.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
)

// ── Message ───────────────────────────────────────────────────────────────────

type Msg struct {
	Type       string `json:"type"`
	MachineID  string `json:"machine_id,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	TargetID   string `json:"target_id,omitempty"`
	ForSession string `json:"for_session,omitempty"`
	FromIP     string `json:"from_ip,omitempty"`
	Data       string `json:"data,omitempty"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	IType      string `json:"itype,omitempty"`
	X          int    `json:"x,omitempty"`
	Y          int    `json:"y,omitempty"`
	Key        string `json:"key,omitempty"`
	Button     int    `json:"button,omitempty"`
	FileID     string `json:"file_id,omitempty"`
	FileName   string `json:"file_name,omitempty"`
	FileSize   int64  `json:"file_size,omitempty"`
	Count      int    `json:"count,omitempty"`
	Error      string `json:"error,omitempty"`
}

// ── Client ────────────────────────────────────────────────────────────────────

var idCounter uint64

type client struct {
	sid       string // unique session ID assigned by relay
	machineID string // set when client registers as host
	conn      *websocket.Conn
	send      chan Msg
	hub       *Hub
	remoteIP  string
}

func (c *client) enqueue(m Msg) {
	select {
	case c.send <- m:
	default:
		log.Printf("[%s] send buffer full, dropping msg type=%s", c.sid, m.Type)
	}
}

func (c *client) writePump() {
	defer c.conn.Close()
	for msg := range c.send {
		if err := c.conn.WriteJSON(msg); err != nil {
			return
		}
	}
}

func (c *client) readPump() {
	defer func() {
		h := c.hub
		h.mu.Lock()
		delete(h.sessions, c.sid)
		if c.machineID != "" {
			delete(h.hosts, c.machineID)
			log.Printf("Host offline: %s", c.machineID)
		}
		// Remove viewer pairings
		for viewerSID, hostMID := range h.viewers {
			if viewerSID == c.sid || hostMID == c.machineID {
				delete(h.viewers, viewerSID)
			}
		}
		h.mu.Unlock()
		close(c.send)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(64 * 1024 * 1024) // 64 MB for large screen frames
	for {
		var msg Msg
		if err := c.conn.ReadJSON(&msg); err != nil {
			break
		}
		c.hub.route(c, msg)
	}
}

// ── Hub ───────────────────────────────────────────────────────────────────────

type Hub struct {
	hosts    map[string]*client // machineID → client
	sessions map[string]*client // sessionID → client
	viewers  map[string]string  // viewer sessionID → host machineID
	mu       sync.RWMutex
}

func newHub() *Hub {
	return &Hub{
		hosts:    make(map[string]*client),
		sessions: make(map[string]*client),
		viewers:  make(map[string]string),
	}
}

func (h *Hub) route(from *client, msg Msg) {
	switch msg.Type {

	// ── Host registers its machine ID ────────────────────────────────────────
	case "register":
		mid := msg.MachineID
		if mid == "" {
			from.enqueue(Msg{Type: "error", Error: "machine_id required"})
			return
		}
		from.machineID = mid
		h.mu.Lock()
		h.hosts[mid] = from
		h.mu.Unlock()
		log.Printf("Host registered: %s (sid=%s ip=%s)", mid, from.sid, from.remoteIP)
		from.enqueue(Msg{Type: "registered", MachineID: mid, SessionID: from.sid})

	// ── Viewer requests to connect to a host ─────────────────────────────────
	case "view_request":
		targetID := msg.TargetID
		h.mu.RLock()
		host, ok := h.hosts[targetID]
		h.mu.RUnlock()
		if !ok {
			from.enqueue(Msg{Type: "error", Error: "Machine " + targetID + " is not online"})
			return
		}
		h.mu.Lock()
		h.viewers[from.sid] = targetID
		h.mu.Unlock()
		host.enqueue(Msg{
			Type:       "incoming_request",
			ForSession: from.sid,
			FromIP:     from.remoteIP,
		})
		log.Printf("View request: %s → host %s", from.sid, targetID)

	// ── Host accepts a viewer ────────────────────────────────────────────────
	case "accept":
		h.mu.RLock()
		viewer, ok := h.sessions[msg.ForSession]
		h.mu.RUnlock()
		if ok {
			viewer.enqueue(Msg{Type: "connected"})
		}

	// ── Host denies a viewer ─────────────────────────────────────────────────
	case "deny":
		h.mu.RLock()
		viewer, ok := h.sessions[msg.ForSession]
		h.mu.RUnlock()
		if ok {
			viewer.enqueue(Msg{Type: "denied"})
			h.mu.Lock()
			delete(h.viewers, msg.ForSession)
			h.mu.Unlock()
		}

	// ── Host → all its viewers (screen frames, status) ───────────────────────
	case "screen_frame", "host_status":
		h.mu.RLock()
		var targets []*client
		for vSID, hMID := range h.viewers {
			if hMID == from.machineID {
				if v, ok := h.sessions[vSID]; ok {
					targets = append(targets, v)
				}
			}
		}
		h.mu.RUnlock()
		for _, v := range targets {
			v.enqueue(msg)
		}

	// ── Viewer → host (input, file transfer) ─────────────────────────────────
	case "input", "file_start", "file_chunk", "file_end":
		h.mu.RLock()
		hostMID, paired := h.viewers[from.sid]
		var host *client
		if paired {
			host = h.hosts[hostMID]
		}
		h.mu.RUnlock()
		if host != nil {
			host.enqueue(msg)
		}

	// ── Viewer disconnects ───────────────────────────────────────────────────
	case "disconnect":
		h.mu.Lock()
		hostMID := h.viewers[from.sid]
		delete(h.viewers, from.sid)
		h.mu.Unlock()
		if hostMID != "" {
			h.mu.RLock()
			host := h.hosts[hostMID]
			h.mu.RUnlock()
			if host != nil {
				host.enqueue(Msg{Type: "host_status", Count: h.viewerCountForHost(hostMID)})
			}
		}
	}
}

func (h *Hub) viewerCountForHost(machineID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	n := 0
	for _, hMID := range h.viewers {
		if hMID == machineID {
			n++
		}
	}
	return n
}

// ── HTTP ─────────────────────────────────────────────────────────────────────

var upgrader = websocket.Upgrader{
	ReadBufferSize:  65536,
	WriteBufferSize: 65536,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func (h *Hub) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}
	n := atomic.AddUint64(&idCounter, 1)
	c := &client{
		sid:      fmt.Sprintf("r%d", n),
		conn:     conn,
		send:     make(chan Msg, 512),
		hub:      h,
		remoteIP: r.RemoteAddr,
	}
	h.mu.Lock()
	h.sessions[c.sid] = c
	h.mu.Unlock()

	// Welcome
	c.enqueue(Msg{Type: "relay_welcome", SessionID: c.sid})

	go c.writePump()
	c.readPump()
}

func main() {
	hub := newHub()

	http.HandleFunc("/ws", hub.handleWS)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		h := hub
		h.mu.RLock()
		hosts := len(h.hosts)
		sessions := len(h.sessions)
		h.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","hosts":%d,"sessions":%d}`, hosts, sessions)
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"service": "Secure System Relay",
			"version": "1.0.0",
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "9090"
	}
	log.Printf("🔒 Secure System Relay listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
