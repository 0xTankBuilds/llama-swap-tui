package api_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"llama-swap-tui/api"
)

func newCaptureTestServer(t *testing.T) (*httptest.Server, *api.Client) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/captures/26112716-a7Kq", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"req_path":"/v1/chat/completions",` +
			`"req_headers":{"Content-Type":"application/json"},` +
			`"req_body":"eyJtZXNzYWdlcyI6W3sicm9sZSI6InVzZXIiLCJjb250ZW50IjoiaGkifV19",` +
			`"resp_headers":{"Content-Type":"application/json"},` +
			`"resp_body":"e30="}`))
	})
	mux.HandleFunc("/api/captures/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "capture not found", http.StatusNotFound)
	})
	mux.HandleFunc("/api/metrics/activity", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":1,"timestamp":"2026-11-27T16:00:00Z","model":"smoke-gemma",` +
			`"req_path":"/v1/chat/completions","resp_status_code":200,"has_capture":true,` +
			`"capture_id":"26112716-a7Kq"}],"page":1,"limit":10,"total":1,"total_pages":1}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, api.NewClient(srv.URL, http.Header{})
}

func TestGetCaptureSuccess(t *testing.T) {
	_, client := newCaptureTestServer(t)
	cap, err := client.GetCapture(context.Background(), "26112716-a7Kq")
	if err != nil {
		t.Fatalf("GetCapture: %v", err)
	}
	if cap.ReqPath != "/v1/chat/completions" {
		t.Errorf("ReqPath = %q, want %q", cap.ReqPath, "/v1/chat/completions")
	}
	if cap.ReqBody == "" || cap.RespBody == "" {
		t.Errorf("bodies empty: req %q resp %q", cap.ReqBody, cap.RespBody)
	}
}

func TestGetCaptureNotFoundIsHTTPError(t *testing.T) {
	_, client := newCaptureTestServer(t)
	_, err := client.GetCapture(context.Background(), "26112716-zzzz")
	if err == nil {
		t.Fatal("GetCapture: want error, got nil")
	}
	var httpErr *api.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("error = %T (%v), want *api.HTTPError", err, err)
	}
	if httpErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want %d", httpErr.StatusCode, http.StatusNotFound)
	}
	if httpErr.Path != "/api/captures/26112716-zzzz" {
		t.Errorf("Path = %q, want %q", httpErr.Path, "/api/captures/26112716-zzzz")
	}
}

func TestGetActivityDecodesCaptureID(t *testing.T) {
	_, client := newCaptureTestServer(t)
	page, err := client.GetActivity(context.Background(), api.ActivityQueryParams{Limit: 10, Order: "desc"})
	if err != nil {
		t.Fatalf("GetActivity: %v", err)
	}
	if len(page.Data) != 1 {
		t.Fatalf("len(Data) = %d, want 1", len(page.Data))
	}
	entry := page.Data[0]
	if !entry.HasCapture {
		t.Error("HasCapture = false, want true")
	}
	if entry.CaptureID != "26112716-a7Kq" {
		t.Errorf("CaptureID = %q, want %q", entry.CaptureID, "26112716-a7Kq")
	}
}
