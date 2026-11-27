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
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
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

// mockCaptureID is the capture ID served for the mock activity row. It
// has the shape the real server generates (UTC yymmddhh + four base58
// characters); any other ID gets a 404, like a pruned capture.
const mockCaptureID = "26112716-a7Kq"

// mockCaptureReqBody is the request body of the canned capture: a chat
// completion whose prompt the smoke test asserts on.
const mockCaptureReqBody = `{"model":"smoke-gemma","messages":[{"role":"user","content":"smoke test prompt"}],"stream":false}`

// mockCaptureRespBody is the response body of the canned capture.
const mockCaptureRespBody = `{"id":"chatcmpl-smoke","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"smoke test reply"},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":8,"total_tokens":20}}`

// mockActivityJSON is the one activity row the mock serves: a chat
// completion with a capture, so the smoke test can open the detail view.
const mockActivityJSON = `{"data":[{"id":1,"timestamp":"2026-11-27T16:00:00Z","src":"proxy","model":"smoke-gemma","req_path":"/v1/chat/completions","resp_content_type":"application/json","resp_status_code":200,"tokens":{"cache_tokens":0,"draft_tokens":0,"draft_acc_tokens":0,"input_tokens":12,"output_tokens":8,"prompt_per_second":120.5,"tokens_per_second":45.2},"duration_ms":320,"has_capture":true,"capture_id":"26112716-a7Kq"}],"page":1,"limit":50,"total":1,"total_pages":1}`

// captureHandler serves the canned capture for mockCaptureID and a 404
// for anything else.
func captureHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/captures/")
	if id != mockCaptureID {
		http.Error(w, "capture not found", http.StatusNotFound)
		return
	}
	body := fmt.Sprintf(`{"id":1,"req_path":"/v1/chat/completions",`+
		`"req_headers":{"Content-Type":"application/json"},`+
		`"req_body":%q,`+
		`"resp_headers":{"Content-Type":"application/json"},`+
		`"resp_body":%q}`,
		base64.StdEncoding.EncodeToString([]byte(mockCaptureReqBody)),
		base64.StdEncoding.EncodeToString([]byte(mockCaptureRespBody)))
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

func main() {
	addr := flag.String("addr", "127.0.0.1:18080", "address to listen on")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/version", writeJSON(`{"version":"1.0.0-mock","commit":"mock","build_date":"2026-11-15"}`))
	mux.HandleFunc("/v1/models", writeJSON(modelsJSON))
	mux.HandleFunc("/api/metrics/activity", writeJSON(mockActivityJSON))
	mux.HandleFunc("/api/captures/", captureHandler)
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
