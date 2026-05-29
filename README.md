# 🔒 Secure System — Remote Desktop

> AnyDesk-style remote desktop for macOS, Windows, and Linux.  
> One binary. Double-click. Share your ID. Done.

---

## ⬇️ Download (Ready-to-Use Binaries)

Go to **[Releases](https://github.com/ShasidharReddy/shasi-remote-desktop/releases/latest)** and download the file for your OS:

| OS | File to Download |
|---|---|
| 🍎 macOS Apple Silicon (M1/M2/M3) | `SecureSystem-mac-arm64` |
| 🍎 macOS Intel | `SecureSystem-mac-intel` |
| 🪟 Windows 64-bit | `SecureSystem-windows-x64.exe` |
| 🐧 Linux x64 | `SecureSystem-linux-x64` |
| 🐧 Linux ARM64 | `SecureSystem-linux-arm64` |

---

## 🚀 Quick Start

### Windows
1. Download `SecureSystem-windows-x64.exe`
2. **Double-click** it — browser opens automatically
3. Your **Machine ID** appears in the browser (e.g. `482-751-293`)
4. Share your Machine ID with whoever needs to connect

### macOS
```bash
# Download, make executable, run
chmod +x SecureSystem-mac-arm64
./SecureSystem-mac-arm64
# Browser opens automatically at http://localhost:8080
```

> **macOS Security Pop-up?** Right-click → Open → Open Anyway  
> (or: System Settings → Privacy & Security → Allow)

### Linux
```bash
chmod +x SecureSystem-linux-x64
./SecureSystem-linux-x64
# Browser opens at http://localhost:8080
```

---

## 🎯 How It Works (AnyDesk-style)

```
Person A (HOST)                    Person B (VIEWER)
─────────────────                  ──────────────────
1. Run SecureSystem                1. Open http://PersonA-IP:8080
2. Gets ID: 482-751-293            2. Enter: 482-751-293
3. Shares ID + IP with B           3. Click "Connect"
4. Sees "B wants to connect"       4. Waits…
5. Clicks "✓ Accept"               5. Sees Person A's screen live!
6. B can now see & control screen  6. Mouse/keyboard sent to Person A
```

---

## 📡 Network Setup

### Same machine (testing)
```
./SecureSystem-mac-arm64
# Open 2 browser tabs at http://localhost:8080
# Tab 1 = host, Tab 2 enters the ID and clicks Connect
```

### Same LAN (home/office network)
```
Person A: ./SecureSystem-mac-arm64 --host 0.0.0.0
Person B: open http://<Person-A-local-IP>:8080

# Find your LAN IP:
# macOS:   ipconfig getifaddr en0
# Windows: ipconfig | findstr IPv4
# Linux:   hostname -I
```

### Custom port
```bash
./SecureSystem-mac-arm64 --port 9090
# Browser at http://localhost:9090
```

---

## 📁 File Transfer

1. Open the **File Transfer** section in the browser UI
2. **Drag & drop** files (or click Browse)
3. Files are chunked (256KB) and sent to the host machine
4. Saved to `~/.shasi-remote/uploads/` on the receiving machine

> Supports files up to **5GB+** — chunked streaming, no size limit.

---

## 🖥️ Screen Capture Support

| OS | Method |
|---|---|
| macOS | `screencapture` command (built-in) ✅ |
| Windows | PowerShell + System.Drawing ✅ |
| Linux | `scrot` or ImageMagick `import` ✅ |

> **macOS**: Grant Screen Recording permission in System Settings → Privacy & Security → Screen Recording

---

## 🔨 Build from Source

### Requirements
- Go 1.21+
- `git clone https://github.com/ShasidharReddy/shasi-remote-desktop`

### Build for your OS
```bash
cd shasi-remote-desktop
go build -o SecureSystem .
./SecureSystem
```

### Build all platforms
```bash
make dist
# Output in dist/:
#   SecureSystem-mac-arm64
#   SecureSystem-mac-intel
#   SecureSystem-windows-x64.exe
#   SecureSystem-linux-x64
#   SecureSystem-linux-arm64
```

### macOS .app Bundle
```bash
make bundle-mac
# Creates dist/Secure System.app — double-click to run
```

---

## 🔧 Troubleshooting

| Problem | Fix |
|---|---|
| Browser doesn't open | Manually go to `http://localhost:8080` |
| "Operation not permitted" on macOS | Allow Screen Recording in System Settings |
| Windows shows "Windows protected your PC" | Click "More info" → "Run anyway" |
| Can't connect from another machine | Check firewall: allow port 8080 TCP |
| Screen is dark/placeholder | Screen recording permission not granted |
| Port 8080 in use | Run with `--port 9090` |

### Logs
```bash
# All logs at:
~/.shasi-remote/remote-desktop.log

tail -f ~/.shasi-remote/remote-desktop.log
```

---

## 📂 Project Structure

```
shasi-remote-desktop/
├── main.go                         # Entry point — starts server + opens browser
├── internal/
│   ├── server/
│   │   ├── relay.go                # WebSocket relay, session management
│   │   ├── capturer.go             # Screen capture + input injection (all OS)
│   │   └── web/                    # Embedded web UI
│   │       ├── index.html          # App shell
│   │       ├── styles.css          # Dark theme styles
│   │       └── app.js              # WebSocket client, viewer/host logic
│   ├── agent/                      # CLI agent mode (legacy)
│   ├── viewer/                     # CLI viewer mode (legacy)
│   ├── screen/                     # Screen capture utilities
│   ├── input/                      # Input controller
│   ├── files/                      # File transfer
│   └── protocol/                   # Message types
├── Makefile                        # Cross-platform build
└── .github/workflows/release.yml  # Auto-release on git tag
```

---

## 🚢 Release a New Version

```bash
git tag v1.1.0
git push origin v1.1.0
# GitHub Actions builds all platforms and creates a release automatically
```

---

## ⚠️ Limitations

- Works best on **LAN** (same network). Cross-internet requires port forwarding.
- Screen capture FPS: ~10fps (limited by OS screenshot tools)
- Input injection on macOS requires Accessibility permissions

