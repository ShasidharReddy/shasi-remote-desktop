# Architecture Overview

## Components

### 1. Relay Server (`internal/server/relay.go`)
- **Purpose**: Central connection hub for agents and viewers
- **Responsibilities**:
  - Accept WebSocket connections
  - Register clients (agents/viewers)
  - Route messages between matching pairs
  - Manage connection lifecycle
  - Provide status endpoint

**Message Flow**:
```
Agent connects → Register(agent_id: "laptop1", role: "agent")
Viewer connects → Register(agent_id: "laptop1", role: "viewer")
Agent → Server → Viewer (screen frames)
Viewer → Server → Agent (input commands)
```

### 2. Agent (`internal/agent/agent.go`)
- **Purpose**: Screen capture and input receiver
- **Responsibilities**:
  - Connect to relay server as "agent"
  - Continuously capture screen frames
  - Receive input commands from viewers
  - Process received files
  - Send compressed screen data

**Loop**:
```
1. Connect to relay server
2. Register as agent
3. Start capture loop (15 FPS)
4. Listen for input/file messages
5. Process and apply inputs locally
```

### 3. Viewer (`internal/viewer/viewer.go`)
- **Purpose**: Display remote screen and send input
- **Responsibilities**:
  - Connect to relay server as "viewer"
  - Receive and cache screen frames
  - Send mouse/keyboard inputs
  - Send files to agent
  - Maintain connection (keepalive)

**Loop**:
```
1. Connect to relay server
2. Register as viewer
3. Receive screen frames
4. Display frames (CLI or GUI)
5. Listen for user input
6. Send input commands to agent
```

### 4. Screen Capture (`internal/screen/capture.go`)
- **Purpose**: Platform-independent screen capture
- **Implementation**: Uses `kbinani/screenshot` library
- **Output**: JPEG-compressed frames
- **Performance**:
  - 15 FPS (configurable)
  - 80/100 JPEG quality
  - ~40-100KB per frame (depending on content)

### 5. Input Controller (`internal/input/controller.go`)
- **Purpose**: Inject mouse and keyboard events
- **Implementation**: Uses `robotn/gohook` library
- **Supported Events**:
  - Mouse move
  - Mouse clicks (left, middle, right)
  - Keyboard key press/release
- **Features**:
  - Input throttling (50ms between events)
  - Key mapping for special keys
  - Cross-platform compatibility

### 6. File Transfer (`internal/files/transfer.go`)
- **Purpose**: Send/receive files between machines
- **Implementation**:
  - 64KB chunks over WebSocket
  - Progress tracking
  - Error handling and retry
- **Flow**:
  ```
  Viewer sends file → FileTransfer message
  Agent opens file for writing
  Viewer sends FileChunk messages
  Agent writes chunks to disk
  Viewer sends FileEnd message
  Agent finalizes and closes file
  ```

## Protocol (JSON over WebSocket)

### Register Message
```json
{
  "type": "register",
  "agent_id": "laptop1",
  "payload": {
    "agent_id": "laptop1",
    "role": "agent"
  }
}
```

### Screen Frame Message
```json
{
  "type": "screen_frame",
  "agent_id": "laptop1",
  "payload": {
    "width": 1920,
    "height": 1080,
    "data": "iVBORw0KGgoAAAANS..."
  }
}
```

### Input Message
```json
{
  "type": "input",
  "agent_id": "laptop1",
  "payload": {
    "type": "mouse_move",
    "x": 500,
    "y": 300
  }
}
```

### File Transfer Messages
```json
{
  "type": "file_transfer",
  "agent_id": "laptop1",
  "payload": {
    "file_name": "document.pdf",
    "file_size": 1048576,
    "file_id": "file_1626451234567"
  }
}
```

## Deployment Scenarios

### Scenario 1: Local Network
```
Laptop A (Agent)     Laptop B (Viewer)
    ↓                     ↓
  [Router] ←→ [Relay Server on Router]
```

### Scenario 2: Remote Access (via SSH Tunnel)
```
Laptop A (Agent)           Laptop B (Viewer)
    ↓                           ↓
[Company VPN] ←→ [Server (Cloud)]
                      ↑
                  SSH Tunnel
```

### Scenario 3: Direct P2P (Future)
```
Laptop A (Agent) ←→ Laptop B (Viewer)
      (No relay needed - nat traversal with STUN/TURN)
```

## Performance Metrics

| Metric | Value | Notes |
|--------|-------|-------|
| Screen FPS | 15 | Configurable, tested to 30 FPS |
| Frame Size | 40-100 KB | JPEG @ 80% quality |
| Bandwidth | 600-1500 KB/s | 15 FPS @ avg 80 KB/frame |
| Input Latency | 50-150ms | Depends on network |
| File Transfer | 5-20 MB/s | LAN speeds |
| Max Concurrent | 1000+ | Per relay server |

## Security Considerations

### Current Implementation
- ⚠️ No authentication (anyone can connect)
- ⚠️ Unencrypted WebSocket
- ✅ Local network only (recommended)

### Production Hardening
- [ ] Add TLS/WSS encryption
- [ ] Implement OAuth2 or JWT auth
- [ ] Add rate limiting
- [ ] Firewall agent IDs
- [ ] Implement permission model (read-only mode)
- [ ] Add audit logging
- [ ] Use certificate pinning

## Future Enhancements

1. **Audio Streaming** - Capture system audio and send to viewer
2. **Clipboard Sync** - Synchronize clipboard between machines
3. **Multi-Monitor** - Support multiple displays
4. **Peer-to-Peer** - Direct connection without relay (STUN/TURN)
5. **Web Viewer** - HTML5 Canvas display in browser
6. **Mobile Client** - iOS/Android apps
7. **Session Recording** - Record sessions for playback
8. **Compression** - H.265/VP9 for better compression
9. **Wake-on-LAN** - Wake machines remotely
10. **VNC Compatibility** - Support existing VNC clients

## Testing Strategy

### Unit Tests
```bash
go test ./internal/protocol
go test ./internal/screen
go test ./internal/input
```

### Integration Tests
```bash
# Start relay server in background
./remote-desktop -mode server &

# Start agent
./remote-desktop -mode agent -agent-id test-1 &

# Start viewer and send commands
./remote-desktop -mode viewer -agent-id test-1
```

### Load Testing
```bash
# Connect multiple agents/viewers
for i in {1..100}; do
  ./remote-desktop -mode agent -agent-id "agent-$i" &
done
```

## Troubleshooting Guide

| Issue | Cause | Solution |
|-------|-------|----------|
| Screen capture fails | Permissions | Grant Screen Recording access |
| Input not working | Library issues | Install `xdotool` (Linux) |
| Slow performance | High resolution | Reduce FPS or JPEG quality |
| Connection timeout | Firewall | Check iptables/Windows Firewall |
| Memory leaks | Old frames cached | Implement frame queue limit |
