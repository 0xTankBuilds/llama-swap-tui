// Command mock-llama-swap is a tiny fake llama-swap server for smoke tests.
//
// It serves the endpoints the TUI hits at boot with canned data, so the
// smoke script can prove a built binary starts, renders, and quits cleanly
// without a real llama-swap instance. The payloads mirror the test fake in
// tui/models_layout_test.go (startFakeAPI), plus the two live streams the
// tests do not need: the SSE event stream and the log stream.
//
// Usage:
//
//	mock-llama-swap -addr 127.0.0.1:18080
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"
)

const modelsJSON = `{"data":[` +
	`{"id":"smoke-gemma","name":"smoke-gemma","description":"Mock model for smoke tests","status":{"value":"loaded"},"context_length":8192},` +
	`{"id":"smoke-llama","name":"smoke-llama","description":"Mock model for smoke tests","status":{"value":"unloaded"},"context_length":4096}` +
	`]}`

// modelStatusEvent is the SSE payload for the modelStatus event: the same
// two models as a []api.Model array, which the TUI parses to set the
// connection state to connected.
const modelStatusEvent = `[{"id":"smoke-gemma","name":"smoke-gemma","description":"Mock model for smoke tests","state":"ready","context_length":8192},` +
	`{"id":"smoke-llama","name":"smoke-llama","description":"Mock model for smoke tests","state":"stopped","context_length":4096}]`

func writeJSON(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

// sseHandler sends one modelStatus event on connect (which marks the TUI
// connected), then a heartbeat comment every 5s to keep the stream alive
// until the client disconnects.
func sseHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	_, _ = fmt.Fprintf(w, "event: modelStatus\ndata: %s\n\n", modelStatusEvent)
	flusher.Flush()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			_, _ = fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

// logStreamHandler replays a few buffered log lines, then holds the
// connection open the way the real /logs/stream tails.
func logStreamHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	for _, line := range []string{
		"data: [mock] llama-swap smoke log line 1\n\n",
		"data: [mock] llama-swap smoke log line 2\n\n",
	} {
		_, _ = fmt.Fprint(w, line)
	}
	flusher.Flush()
	<-r.Context().Done()
}

func main() {
	addr := flag.String("addr", "127.0.0.1:18080", "address to listen on")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/version", writeJSON(`{"version":"1.0.0-mock","commit":"mock","build_date":"2026-11-15"}`))
	mux.HandleFunc("/v1/models", writeJSON(modelsJSON))
	mux.HandleFunc("/api/metrics/activity", writeJSON(`{"data":[],"page":1,"limit":50,"total":0,"total_pages":0}`))
	mux.HandleFunc("/api/metrics/stats", writeJSON(`{}`))
	mux.HandleFunc("/api/hardware", writeJSON(`{}`))
	mux.HandleFunc("/api/profiles", writeJSON(`{"active":"default","profiles":[{"id":"default","description":"Mock default profile","pins":{}}]}`))
	mux.HandleFunc("/api/performance", writeJSON(`{"sys_stats":[{"timestamp":"t1","mem_total_mb":100,"mem_used_mb":10}],"gpu_stats":[{"timestamp":"t1","id":0,"name":"T","gpu_util_pct":10}]}`))
	mux.HandleFunc("/api/tailcat", writeJSON(`{}`))
	mux.HandleFunc("/api/events", sseHandler)
	mux.HandleFunc("/logs/stream", logStreamHandler)

	log.Printf("mock llama-swap listening on %s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatalf("mock llama-swap: %v", err)
	}
}
