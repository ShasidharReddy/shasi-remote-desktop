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

func main() {
	port := flag.String("port", "8080", "Port to listen on")
	host := flag.String("host", "0.0.0.0", "Interface to listen on")
	flag.Parse()

	setupLogging()

	srv := server.NewRelayServer(*host, *port)

	url := fmt.Sprintf("http://localhost:%s", *port)
	go func() {
		time.Sleep(600 * time.Millisecond)
		openBrowser(url)
	}()

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════╗")
	fmt.Println("║   🔒  Secure System  v1.0             ║")
	fmt.Printf("║   Open: %-28s ║\n", url)
	fmt.Println("╚══════════════════════════════════════╝")
	fmt.Printf("\n   Your Machine ID: %s\n\n", srv.MachineID())
	fmt.Println("   Share your Machine ID + this URL with the person")
	fmt.Println("   who wants to view/control your screen.")
	fmt.Println()
	fmt.Println("   Press Ctrl+C to stop.")
	fmt.Println()

	if err := srv.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
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
