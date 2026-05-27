package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"

	"github.com/ShasidharReddy/shasi-remote-desktop/internal/agent"
	"github.com/ShasidharReddy/shasi-remote-desktop/internal/server"
	"github.com/ShasidharReddy/shasi-remote-desktop/internal/viewer"
)

func main() {
	mode := flag.String("mode", "server", "Mode: server, agent, or viewer")
	host := flag.String("host", "localhost", "Host to listen on (server mode) or connect to (viewer/agent)")
	port := flag.String("port", "8080", "Port to listen on (server mode) or connect to (viewer/agent)")
	agentID := flag.String("agent-id", "", "Agent ID (agent or viewer mode)")
	flag.Parse()

	logFile := openLogFile()
	defer logFile.Close()
	log.SetOutput(logFile)

	log.Printf("=== Shasi Remote Desktop v1.0 ===")
	log.Printf("Mode: %s, Host: %s, Port: %s, AgentID: %s", *mode, *host, *port, *agentID)

	switch *mode {
	case "server":
		runServer(*host, *port)
	case "agent":
		if *agentID == "" {
			log.Fatal("Agent mode requires --agent-id")
		}
		runAgent(*host, *port, *agentID)
	case "viewer":
		if *agentID == "" {
			log.Fatal("Viewer mode requires --agent-id")
		}
		runViewer(*host, *port, *agentID)
	default:
		fmt.Fprintf(os.Stderr, "Unknown mode: %s\n", *mode)
		os.Exit(1)
	}
}

func runServer(host, port string) {
	addr := net.JoinHostPort(host, port)
	log.Printf("Starting relay server on %s", addr)
	srv := server.NewRelayServer(addr)
	if err := srv.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func runAgent(host, port, agentID string) {
	addr := net.JoinHostPort(host, port)
	log.Printf("Starting agent (ID: %s), connecting to server at %s", agentID, addr)
	ag := agent.NewAgent(agentID, addr)
	if err := ag.Connect(); err != nil {
		log.Fatalf("Agent error: %v", err)
	}
}

func runViewer(host, port, agentID string) {
	addr := net.JoinHostPort(host, port)
	log.Printf("Starting viewer for agent %s, connecting to server at %s", agentID, addr)
	v := viewer.NewViewer(agentID, addr)
	if err := v.Connect(); err != nil {
		log.Fatalf("Viewer error: %v", err)
	}
}

func openLogFile() *os.File {
	logDir := filepath.Join(os.ExpandEnv("$HOME"), ".shasi-remote")
	os.MkdirAll(logDir, 0755)
	logFile, err := os.OpenFile(
		filepath.Join(logDir, "remote-desktop.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0644,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open log file: %v\n", err)
		os.Exit(1)
	}
	return logFile
}
