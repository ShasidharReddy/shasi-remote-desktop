package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/ShasidharReddy/shasi-remote-desktop/internal/server"
)

// Default public relay — users can override with RELAY_URL env var.
// Set to empty to run in LAN-only mode.
const defaultRelayURL = "wss://secure-system-relay.onrender.com/ws"

func main() {
	port     := flag.String("port", "8080", "Local port for browser UI")
	host     := flag.String("host", "0.0.0.0", "Interface to listen on")
	relayURL := flag.String("relay", getEnvOrDefault("RELAY_URL", defaultRelayURL), "Cloud relay WebSocket URL (empty = LAN only)")
	flag.Parse()

	setupLogging()

	srv := server.NewRelayServer(*host, *port)

	// Connect to cloud relay so machines on different networks can find each other
	if *relayURL != "" {
		go srv.ConnectRelay(*relayURL)
	}

	url := fmt.Sprintf("http://localhost:%s", *port)
	go func() {
		time.Sleep(700 * time.Millisecond)
		openBrowser(url)
	}()

	mode := "LAN only"
	if *relayURL != "" {
		mode = "Internet (via relay)"
	}

	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════╗")
	fmt.Println("║   🔒  Secure System  v1.0                   ║")
	fmt.Printf( "║   Open:  %-33s║\n", url)
	fmt.Printf( "║   Mode:  %-33s║\n", mode)
	fmt.Println("╚════════════════════════════════════════════╝")
	fmt.Printf("\n   Your Machine ID: %s\n\n", srv.MachineID())
	fmt.Println("   Share your Machine ID with the person who wants to connect.")
	fmt.Println("   They just paste it in the 'Connect to Remote' box.")
	fmt.Println()
	fmt.Println("   Press Ctrl+C to stop.")
	fmt.Println()

	if err := srv.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func openBrowser(url string) {
	switch runtime.GOOS {
	case "darwin":
		exec.Command("open", url).Start()
	case "linux":
		exec.Command("xdg-open", url).Start()
	case "windows":
		exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	}
}

func setupLogging() {
	logDir := filepath.Join(os.ExpandEnv("$HOME"), ".shasi-remote")
	os.MkdirAll(logDir, 0755)
	logFile, err := os.OpenFile(
		filepath.Join(logDir, "remote-desktop.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0644,
	)
	if err != nil {
		return
	}
	log.SetOutput(logFile)
}
