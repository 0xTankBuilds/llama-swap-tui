// llama-swap-tui — a terminal UI for llama-swap.
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"llama-swap-tui/api"
	"llama-swap-tui/tui"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	version     = "v1.3.0-auto-refresh"
	defaultHost = "http://localhost:8080"
	envKey      = "LLAMA_SWAP_URL"
)

func main() {
	host := defaultHost

	if idx := flagIndex("--host", os.Args[1:]); idx >= 0 {
		if idx+1 < len(os.Args[1:]) {
			host = os.Args[1:][idx+1]
		} else {
			log.Fatalf("Error: --host requires a value\n")
		}
	}
	if host == defaultHost {
		if env := os.Getenv(envKey); env != "" {
			host = env
		}
	}

	log.SetFlags(0)
	log.SetOutput(os.Stderr)

	client := api.NewClient(host, nil)

	model := tui.NewModel(client, version)

	program := tea.NewProgram(model, tea.WithAltScreen())

	// Wire the program reference to the model so goroutines
	// (SSE reader, log stream) can send messages into the update loop.
	model.SetProgram(program)

	// Handle signals for graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		program.Send(tui.ShutdownMsg{})
	}()

	if _, err := program.Run(); err != nil {
		log.Fatalf("TUI error: %v\n", err)
	}
}

// flagIndex returns the index of a flag in args, or -1 if not found.
func flagIndex(name string, args []string) int {
	for i, arg := range args {
		if arg == name {
			return i
		}
	}
	return -1
}
