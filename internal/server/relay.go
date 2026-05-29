package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

//go:embed web
var webFS embed.FS

// ─── message types ────────────────────────────────────────────────────────────

const (
	msgWelcome         = "welcome"
	msgRegisterHost    = "register_host"
	msgViewRequest     = "view_request"
	msgIncomingRequest = "incoming_request"
	msgAccept          = "accept"
	msgDeny            = "deny"
	msgConnected       = "connected"
	msgDenied          = "denied"
	msgScreenFrame     = "screen_frame"
	msgInput           = "input"
	msgFileStart       = "file_start"
	msgFileChunk       = "file_chunk"
	msgFileEnd         = "file_end"
	msgDisconnect      = "disconnect"
	msgHostStatus      = "host_status"
	msgError           = "error"
)

type WsMsg struct {
	Type       string `json:"type"`
	SessionID  string `json:"session_id,omitempty"`
	MachineID  string `json:"machine_id,omitempty"`
	ForSession string `json:"for_session,omitempty"`
	TargetID   string `json:"target_id,omitempty"`
	ConnAddr   string `json:"conn_addr,omitempty"` // LAN IP:port sent in welcome
	FromIP     string `json:"from_ip,omitempty"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	Data       string `json:"data,omitempty"`
	IType      string `json:"itype,omitempty"`
	X          int    `json:"x,omitempty"`
	Y          int    `json:"y,omitempty"`
	Button     int    `json:"button,omitempty"`
	Key        string `json:"key,omitempty"`
	FileID     string `json:"file_id,omitempty"`
	FileName   string `json:"file_name,omitempty"`
	FileSize   int64  `json:"file_size,omitempty"`
	Count      int    `json:"count,omitempty"`
	Err        string `json:"error,omitempty"`
}

// ─── session ──────────────────────────────────────────────────────────────────

type session struct {
	id       string
	conn     *websocket.Conn
	role     string // "pending" | "host" | "viewer_pending" | "viewer"
	remoteIP string
	send     chan WsMsg
	writeMu  sync.Mutex
}

func (s *session) enqueue(m WsMsg) {
	select {
	case s.send <- m:
	default:
	}
}

// ─── relay server ─────────────────────────────────────────────────────────────

type RelayServer struct {
	host      string
	port      string
	machineID string
	localIP   string
	sessions  map[string]*session
	mu        sync.RWMutex
	upgrader  websocket.Upgrader
	cap       *capturer
	uploadDir string
	streamCh  chan struct{}
	streamMu  sync.Mutex

	// Cloud relay connection (nil = LAN-only mode)
	relayConn   *websocket.Conn
	relayMu     sync.Mutex
	relayOnline bool
}

func NewRelayServer(host, port string) *RelayServer {
	homeDir, _ := os.UserHomeDir()
	uploadDir := filepath.Join(homeDir, ".shasi-remote", "uploads")
	os.MkdirAll(uploadDir, 0755)

	return &RelayServer{
		host:      host,
		port:      port,
		machineID: loadOrGenMachineID(),
		localIP:   getLANIP(),
		sessions:  make(map[string]*session),
		upgrader:  websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
		cap:       newCapturer(),
		uploadDir: uploadDir,
	}
}

func (s *RelayServer) MachineID() string { return s.machineID }

func (s *RelayServer) Start() error {
	subFS, err := fs.Sub(webFS, "web")
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWS)
	mux.HandleFunc("/status", s.handleStatus)
	mux.Handle("/", http.FileServer(http.FS(subFS)))

	addr := net.JoinHostPort(s.host, s.port)
	log.Printf("Relay server listening on %s | Machine ID: %s", addr, s.machineID)
	return http.ListenAndServe(addr, mux)
}

// ─── websocket handler ────────────────────────────────────────────────────────

func (s *RelayServer) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}

	sess := &session{
		id:       genID(8),
		conn:     conn,
		role:     "pending",
		remoteIP: r.RemoteAddr,
		send:     make(chan WsMsg, 512),
	}

	s.mu.Lock()
	s.sessions[sess.id] = sess
	s.mu.Unlock()

	// greet with machine ID + LAN address so browser knows how to share
	sess.enqueue(WsMsg{
		Type:      msgWelcome,
		SessionID: sess.id,
		MachineID: s.machineID,
		ConnAddr:  s.localIP + ":" + s.port,
	})

	go s.writePump(sess)
	s.readPump(sess)
}

func (s *RelayServer) readPump(sess *session) {
	defer func() {
		s.mu.Lock()
		delete(s.sessions, sess.id)
		s.mu.Unlock()
		close(sess.send)
		sess.conn.Close()

		if sess.role == "viewer" && s.viewerCount() == 0 {
			s.stopScreenStream()
		}
		s.broadcastToHosts(WsMsg{Type: msgHostStatus, Count: s.viewerCount()})
		log.Printf("Session gone: %s (%s)", sess.id, sess.role)
	}()

	for {
		var msg WsMsg
		if err := sess.conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("Read error [%s]: %v", sess.id, err)
			}
			return
		}
		s.dispatch(sess, msg)
	}
}

func (s *RelayServer) writePump(sess *session) {
	defer sess.conn.Close()
	for msg := range sess.send {
		sess.writeMu.Lock()
		err := sess.conn.WriteJSON(msg)
		sess.writeMu.Unlock()
		if err != nil {
			log.Printf("Write error [%s]: %v", sess.id, err)
			return
		}
	}
}

// ─── message dispatch ─────────────────────────────────────────────────────────

func (s *RelayServer) dispatch(sess *session, msg WsMsg) {
	switch msg.Type {

	case msgRegisterHost:
		sess.role = "host"
		log.Printf("Host registered: %s from %s", sess.id, sess.remoteIP)

	case msgViewRequest:
		// Verify this request is for this machine (local mode)
		if msg.TargetID != "" && msg.TargetID != s.machineID {
			// In relay mode — forward to cloud relay so it can route to correct machine
			s.relayMu.Lock()
			online := s.relayOnline
			s.relayMu.Unlock()
			if online && sess != nil {
				// Create a virtual proxy session so relay responses come back here
				proxyKey := "relay:" + sess.id
				s.mu.Lock()
				s.sessions[proxyKey] = sess
				s.mu.Unlock()
				s.sendToRelay(WsMsg{
					Type:      msgViewRequest,
					TargetID:  msg.TargetID,
					SessionID: sess.id,
				})
				sess.enqueue(WsMsg{Type: "relay_routing"}) // ack
				return
			}
			if sess != nil {
				sess.enqueue(WsMsg{Type: msgError, Err: "Machine ID not found — is the other machine online?"})
			}
			return
		}
		if sess != nil {
			sess.role = "viewer_pending"
		}
		log.Printf("View request from %s (%s)", func() string {
			if sess != nil {
				return sess.id
			}
			return "relay"
		}(), func() string {
			if sess != nil {
				return sess.remoteIP
			}
			return "relay"
		}())
		fromSID := ""
		fromIP := ""
		if sess != nil {
			fromSID = sess.id
			fromIP = sess.remoteIP
		} else {
			fromSID = msg.SessionID
			fromIP = "relay"
		}
		s.broadcastToHosts(WsMsg{
			Type:       msgIncomingRequest,
			SessionID:  fromSID,
			ForSession: fromSID,
			FromIP:     fromIP,
		})

	case msgAccept:
		forSID := msg.ForSession
		s.mu.RLock()
		target, ok := s.sessions[forSID]
		s.mu.RUnlock()
		if ok {
			target.role = "viewer"
			target.enqueue(WsMsg{Type: msgConnected})
			log.Printf("Accepted viewer: %s", target.id)
			s.startScreenStream()
			s.broadcastToHosts(WsMsg{Type: msgHostStatus, Count: s.viewerCount()})
		} else {
			// Viewer is on the relay — forward accept through relay
			s.sendToRelay(WsMsg{Type: msgAccept, ForSession: forSID})
			// Start streaming to relay
			s.startRelayStream(forSID)
		}

	case msgDeny:
		forSID := msg.ForSession
		s.mu.RLock()
		target, ok := s.sessions[forSID]
		s.mu.RUnlock()
		if ok {
			target.role = "denied"
			target.enqueue(WsMsg{Type: msgDenied})
		} else {
			s.sendToRelay(WsMsg{Type: msgDeny, ForSession: forSID})
		}

	case msgInput:
		if sess.role != "viewer" {
			return
		}
		s.cap.executeInput(msg)

	case msgFileStart:
		s.cap.startFile(msg.FileID, msg.FileName, msg.FileSize, s.uploadDir)

	case msgFileChunk:
		s.cap.writeChunk(msg.FileID, msg.Data)

	case msgFileEnd:
		if path := s.cap.finishFile(msg.FileID); path != "" {
			s.broadcastToHosts(WsMsg{
				Type:     msgFileEnd,
				FileID:   msg.FileID,
				FileName: filepath.Base(path),
			})
		}

	case msgDisconnect:
		sess.role = "pending"
		s.broadcastToHosts(WsMsg{Type: msgHostStatus, Count: s.viewerCount()})
		if s.viewerCount() == 0 {
			s.stopScreenStream()
		}
	}
}

// ─── screen streaming ─────────────────────────────────────────────────────────

func (s *RelayServer) startScreenStream() {
	s.streamMu.Lock()
	defer s.streamMu.Unlock()
	if s.streamCh != nil {
		return
	}
	s.streamCh = make(chan struct{})
	go s.streamLoop(s.streamCh)
}

func (s *RelayServer) stopScreenStream() {
	s.streamMu.Lock()
	defer s.streamMu.Unlock()
	if s.streamCh != nil {
		close(s.streamCh)
		s.streamCh = nil
	}
}

func (s *RelayServer) streamLoop(quit chan struct{}) {
	ticker := time.NewTicker(100 * time.Millisecond) // ~10 fps
	defer ticker.Stop()
	for {
		select {
		case <-quit:
			return
		case <-ticker.C:
			if s.viewerCount() == 0 && !s.relayOnline {
				s.stopScreenStream()
				return
			}
			b64, w, h, err := s.cap.capture()
			if err != nil {
				continue
			}
			frame := WsMsg{Type: msgScreenFrame, Width: w, Height: h, Data: b64}
			// Send to local browser viewers
			s.broadcastToViewers(frame)
			// Also send to relay (for remote viewers connected via cloud)
			s.sendToRelay(frame)
		}
	}
}

// ─── broadcast helpers ────────────────────────────────────────────────────────

func (s *RelayServer) broadcastToHosts(msg WsMsg) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sess := range s.sessions {
		if sess.role == "host" {
			sess.enqueue(msg)
		}
	}
}

func (s *RelayServer) broadcastToViewers(msg WsMsg) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sess := range s.sessions {
		if sess.role == "viewer" {
			sess.enqueue(msg)
		}
	}
}

func (s *RelayServer) viewerCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, sess := range s.sessions {
		if sess.role == "viewer" {
			n++
		}
	}
	return n
}

// ─── /status endpoint ────────────────────────────────────────────────────────

func (s *RelayServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	type info struct {
		ID   string `json:"id"`
		Role string `json:"role"`
		IP   string `json:"ip"`
	}
	list := make([]info, 0, len(s.sessions))
	for _, sess := range s.sessions {
		list = append(list, info{ID: sess.id, Role: sess.role, IP: sess.remoteIP})
	}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"machine_id": s.machineID,
		"sessions":   list,
		"viewers":    s.viewerCount(),
	})
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func loadOrGenMachineID() string {
	homeDir, _ := os.UserHomeDir()
	idFile := filepath.Join(homeDir, ".shasi-remote", "machine-id")
	os.MkdirAll(filepath.Dir(idFile), 0755)
	if data, err := os.ReadFile(idFile); err == nil && len(data) == 11 {
		return string(data)
	}
	id := fmt.Sprintf("%03d-%03d-%03d",
		rand.Intn(900)+100,
		rand.Intn(900)+100,
		rand.Intn(900)+100,
	)
	os.WriteFile(idFile, []byte(id), 0644)
	return id
}

func genID(n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}

// getLANIP returns the first non-loopback IPv4 address found on the system.
func getLANIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "localhost"
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			if ip4 := ip.To4(); ip4 != nil {
				return ip4.String()
			}
		}
	}
	return "localhost"
}

// ─── cloud relay client ───────────────────────────────────────────────────────
// ConnectRelay connects to the cloud relay server and keeps the connection alive.
// It registers this machine's ID so remote viewers can find us.
// All viewer messages from the relay are forwarded to local browser sessions.

func (s *RelayServer) ConnectRelay(relayURL string) {
	backoff := 2
	for {
		log.Printf("Connecting to relay: %s", relayURL)
		conn, _, err := websocket.DefaultDialer.Dial(relayURL, nil)
		if err != nil {
			log.Printf("Relay connection failed: %v — retrying in %ds", err, backoff)
			time.Sleep(time.Duration(backoff) * time.Second)
			if backoff < 60 {
				backoff *= 2
			}
			continue
		}
		backoff = 2
		s.relayMu.Lock()
		s.relayConn = conn
		s.relayOnline = true
		s.relayMu.Unlock()
		log.Printf("Relay connected ✓  machine-id=%s", s.machineID)

		// Register this machine with the relay
		s.sendToRelay(WsMsg{Type: "register", MachineID: s.machineID})

		// Broadcast relay status to local browser sessions
		s.broadcastToHosts(WsMsg{Type: "relay_status", Data: "online"})

		// Read messages from relay and forward to local browser
		s.relayReadLoop(conn)

		s.relayMu.Lock()
		s.relayConn = nil
		s.relayOnline = false
		s.relayMu.Unlock()

		s.broadcastToHosts(WsMsg{Type: "relay_status", Data: "offline"})
		log.Printf("Relay disconnected — reconnecting in %ds", backoff)
		time.Sleep(time.Duration(backoff) * time.Second)
	}
}

// relayReadLoop handles messages coming FROM the cloud relay.
func (s *RelayServer) relayReadLoop(conn *websocket.Conn) {
	conn.SetReadLimit(64 * 1024 * 1024)
	for {
		var msg WsMsg
		if err := conn.ReadJSON(&msg); err != nil {
			log.Printf("Relay read error: %v", err)
			return
		}
		switch msg.Type {
		case "relay_welcome", "registered":
			// Relay confirmed our registration — nothing to do

		case "incoming_request":
			// A remote viewer wants to connect — tell local browser host sessions
			s.broadcastToHosts(msg)

		case "connected":
			// Relay-initiated viewer got accepted — forward to that viewer's local proxy session
			s.forwardToRelayViewer(msg.SessionID, msg)

		case "denied":
			s.forwardToRelayViewer(msg.SessionID, msg)

		case "screen_frame", "host_status":
			// Viewer on relay side receives screen — forward to their local browser
			s.forwardToRelayViewer(msg.SessionID, msg)

		case "input", "file_start", "file_chunk", "file_end":
			// Remote viewer sending input/files — execute locally
			s.dispatch(nil, msg)

		case "error":
			log.Printf("Relay error: %s", msg.Err)
		}
	}
}

// startRelayStream starts screen capture that sends frames to the cloud relay.
func (s *RelayServer) startRelayStream(viewerSessionID string) {
	s.startScreenStream() // reuses same stream loop — broadcastToViewers + relay
}

func (s *RelayServer) sendToRelay(msg WsMsg) {
	s.relayMu.Lock()
	conn := s.relayConn
	s.relayMu.Unlock()
	if conn == nil {
		return
	}
	if err := conn.WriteJSON(msg); err != nil {
		log.Printf("Relay send error: %v", err)
	}
}

// forwardToRelayViewer finds the local proxy session for a relay-viewer and sends them msg.
// When a viewer connects via cloud relay, we create a virtual proxy session for them.
func (s *RelayServer) forwardToRelayViewer(viewerSessionID string, msg WsMsg) {
	s.mu.RLock()
	sess, ok := s.sessions["relay:"+viewerSessionID]
	s.mu.RUnlock()
	if ok {
		sess.enqueue(msg)
	}
}

