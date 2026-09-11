// Package api provides a client for the llama-swap HTTP API and SSE event stream.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// SSE reader
// ---------------------------------------------------------------------------

// SSEEvent represents a parsed Server-Sent Event.
type SSEEvent struct {
	Type string
	Data string
}

// SSEClient reads SSE events from a URL. Call Start() to begin reading.
// Each parsed event is sent on the Events channel.
type SSEClient struct {
	url     string
	events  chan SSEEvent
	errCh   chan error
	httpCli *http.Client
	mu      sync.Mutex
	done    chan struct{}
	header  http.Header
}

// NewSSEClient creates a new SSE reader. Call Start() to begin.
func NewSSEClient(url string, header http.Header) *SSEClient {
	return &SSEClient{
		url:     strings.TrimRight(url, "/") + "/api/events",
		events:  make(chan SSEEvent, 256),
		errCh:   make(chan error, 1),
		httpCli: &http.Client{Timeout: 0}, // no timeout; context-driven
		done:    make(chan struct{}),
		header:  header,
	}
}

// Events returns the channel that SSE events are delivered on.
func (c *SSEClient) Events() <-chan SSEEvent { return c.events }

// Errors returns the channel that connection errors are delivered on.
func (c *SSEClient) Errors() <-chan error { return c.errCh }

// Start begins reading the SSE stream in a background goroutine.
func (c *SSEClient) Start(ctx context.Context) {
	go c.readLoop(ctx)
}

// Close stops the reader and closes the events channel.
func (c *SSEClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case <-c.done:
	default:
		close(c.done)
	}
}

func (c *SSEClient) readLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		default:
		}

		req, err := http.NewRequestWithContext(ctx, "GET", c.url, nil)
		if err != nil {
			c.errCh <- fmt.Errorf("sse: create request: %w", err)
			time.Sleep(1 * time.Second)
			continue
		}
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Cache-Control", "no-cache")
		for k, vals := range c.header {
			for _, v := range vals {
				req.Header.Add(k, v)
			}
		}

		resp, err := c.httpCli.Do(req)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-c.done:
				return
			case c.errCh <- fmt.Errorf("sse: request failed: %w", err):
			}
			time.Sleep(1 * time.Second)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			c.errCh <- fmt.Errorf("sse: unexpected status %d", resp.StatusCode)
			time.Sleep(1 * time.Second)
			continue
		}

		// Read events until the connection is closed or context is done.
		buf := make([]byte, 0, 16384)
		var eventType string
		var dataBuf strings.Builder

		for {
			b := make([]byte, 1024)
			n, readErr := resp.Body.Read(b)
			buf = append(buf, b[:n]...)

			// Process complete lines from the buffer.
			for {
				idx := bytes.IndexByte(buf, '\n')
				if idx == -1 {
					break
				}
				line := string(buf[:idx])
				buf = buf[idx+1:]

				// Handle \r\n or bare \n.
				line = strings.TrimRight(line, "\r")

				if line == "" {
					// Empty line signals end of an event.
					if dataBuf.Len() > 0 {
						event := SSEEvent{
							Type: eventType,
							Data: dataBuf.String(),
						}
						select {
						case c.events <- event:
						case <-c.done:
						}
					}
					eventType = ""
					dataBuf.Reset()
				} else if strings.HasPrefix(line, "event:") {
					eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
				} else if strings.HasPrefix(line, "data:") {
					dataBuf.WriteString(strings.TrimPrefix(line, "data:") + "\n")
				} else if strings.HasPrefix(line, "id:") {
					// Event ID — skip for now.
				} else {
					// Unknown field, skip.
				}
			}

			if readErr != nil {
				if readErr == io.EOF {
					break
				}
				select {
				case <-ctx.Done():
					resp.Body.Close()
					return
				case <-c.done:
					resp.Body.Close()
					return
				default:
				}
			}
		}

		resp.Body.Close()

		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		default:
		}

		// Reconnect with backoff.
		time.Sleep(1 * time.Second)
	}
}

// ---------------------------------------------------------------------------
// REST client
// ---------------------------------------------------------------------------

// Client is the high-level llama-swap API client.
type Client struct {
	BaseURL string
	Header  http.Header
	httpCli *http.Client
}

// NewClient creates a new API client.
func NewClient(baseURL string, header http.Header) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Header:  header,
		httpCli: &http.Client{Timeout: 30 * time.Second},
	}
}

// doJSON performs a GET request and decodes the JSON response into v.
func (c *Client) doJSON(ctx context.Context, path string, v interface{}) error {
	url := c.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("request %s: %w", path, err)
	}
	for k, vals := range c.Header {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}

	resp, err := c.httpCli.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d", path, resp.StatusCode)
	}

	if v != nil {
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			return fmt.Errorf("decode %s: %w", path, err)
		}
	}
	return nil
}

// doJSONPost performs a POST request with a JSON body and decodes the response.
func (c *Client) doJSONPost(ctx context.Context, path string, body interface{}, v interface{}) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		r = bytes.NewReader(b)
	}

	url := c.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, "POST", url, r)
	if err != nil {
		return fmt.Errorf("request %s: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, vals := range c.Header {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}

	resp, err := c.httpCli.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s: HTTP %d: %s", path, resp.StatusCode, string(bodyBytes))
	}

	if v != nil {
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			return fmt.Errorf("decode %s: %w", path, err)
		}
	}
	return nil
}

// doJSONPut performs a PUT request with a JSON body and decodes the response.
func (c *Client) doJSONPut(ctx context.Context, path string, body interface{}, v interface{}) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		r = bytes.NewReader(b)
	}

	url := c.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, "PUT", url, r)
	if err != nil {
		return fmt.Errorf("request %s: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, vals := range c.Header {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}

	resp, err := c.httpCli.Do(req)
	if err != nil {
		return fmt.Errorf("PUT %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PUT %s: HTTP %d: %s", path, resp.StatusCode, string(bodyBytes))
	}

	if v != nil {
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			return fmt.Errorf("decode %s: %w", path, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// REST API methods
// ---------------------------------------------------------------------------

// GetActivity fetches paginated activity from /api/metrics/activity.
func (c *Client) GetActivity(ctx context.Context, params ActivityQueryParams) (*ActivityPage, error) {
	q := url.Values{}
	if len(params.Models) > 0 {
		for _, m := range params.Models {
			q.Add("model", m)
		}
	}
	if params.Page > 0 {
		q.Set("page", strconv.Itoa(params.Page))
	}
	if params.Limit > 0 {
		q.Set("limit", strconv.Itoa(params.Limit))
	}
	if params.Sort != "" {
		q.Set("sort", params.Sort)
	}
	if params.Order != "" {
		q.Set("order", params.Order)
	}
	if params.StartID > 0 {
		q.Set("start_id", strconv.Itoa(params.StartID))
	}
	if params.EndID > 0 {
		q.Set("end_id", strconv.Itoa(params.EndID))
	}
	if params.SrcPrefix != "" {
		q.Set("src_prefix", params.SrcPrefix)
	}
	url := "/api/metrics/activity"
	if len(q) > 0 {
		url += "?" + q.Encode()
	}

	var result ActivityPage
	if err := c.doJSON(ctx, url, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ActivityQueryParams builds query parameters for GetActivity.
type ActivityQueryParams struct {
	Models    []string
	Page      int
	Limit     int
	Sort      string
	Order     string // "asc" or "desc"
	StartID   int
	EndID     int
	SrcPrefix string
}

// GetActivityStats fetches aggregate stats from /api/metrics/stats.
func (c *Client) GetActivityStats(ctx context.Context, model string) (*ActivityStats, error) {
	url := "/api/metrics/stats"
	if model != "" {
		url += "?model=" + model
	}
	var result ActivityStats
	if err := c.doJSON(ctx, url, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetModels fetches the model list from /v1/models and enriches it with
// llama-swap metadata. Returns the models sorted by name.
func (c *Client) GetModels(ctx context.Context) ([]Model, error) {
	var resp struct {
		Data []struct {
			ID          string                 `json:"id"`
			Name        string                 `json:"name"`
			Description string                 `json:"description"`
			Capabilities *ModelCapabilities  `json:"capabilities,omitempty"`
			ContextLength int                  `json:"context_length,omitempty"`
			Meta         map[string]any         `json:"meta"`
		} `json:"data"`
	}
	if err := c.doJSON(ctx, "/v1/models", &resp); err != nil {
		return nil, err
	}

	models := make([]Model, 0, len(resp.Data))
	for _, d := range resp.Data {
		m := Model{
			ID:            d.ID,
			Name:          d.Name,
			Description:   d.Description,
			Capabilities:  d.Capabilities,
			ContextLength: d.ContextLength,
		}
		if meta, ok := d.Meta["llamaswap"].(map[string]any); ok {
			if aliases, ok := meta["aliases"].([]any); ok {
				m.Aliases = make([]string, 0, len(aliases))
				for _, a := range aliases {
					if s, ok := a.(string); ok {
						m.Aliases = append(m.Aliases, s)
					}
				}
			}
			if targets, ok := meta["targets"].([]any); ok {
				m.Targets = make([]string, 0, len(targets))
				for _, t := range targets {
					if s, ok := t.(string); ok {
						m.Targets = append(m.Targets, s)
					}
				}
			}
			if sp, ok := meta["spillover"].(float64); ok {
				m.Spillover = int(sp)
			}
			if strat, ok := meta["strategy"].(string); ok {
				m.Strategy = strat
			}
		}
		models = append(models, m)
	}
	return models, nil
}

// GetProfiles fetches profile list from /api/profiles.
func (c *Client) GetProfiles(ctx context.Context) (*ProfileState, error) {
	var result ProfileState
	if err := c.doJSON(ctx, "/api/profiles", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SetActiveProfile switches the active profile via PUT /api/profiles/active.
func (c *Client) SetActiveProfile(ctx context.Context, name *string) (*ProfileState, error) {
	var result ProfileState
	payload := map[string]*string{"name": name}
	if err := c.doJSONPut(ctx, "/api/profiles/active", payload, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetVersion fetches version info from /api/version.
func (c *Client) GetVersion(ctx context.Context) (*VersionInfo, error) {
	var result VersionInfo
	if err := c.doJSON(ctx, "/api/version", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetHardware fetches hardware snapshot from /api/hardware.
func (c *Client) GetHardware(ctx context.Context) (*HardwareSnapshot, error) {
	var result HardwareSnapshot
	if err := c.doJSON(ctx, "/api/hardware", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetPerformance fetches historical performance data from /api/performance.
func (c *Client) GetPerformance(ctx context.Context, after string) (*PerformanceResponse, error) {
	url := "/api/performance"
	if after != "" {
		url += "?after=" + after
	}
	var result PerformanceResponse
	if err := c.doJSON(ctx, url, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetTailcatStatus fetches tailcat status from /api/tailcat.
func (c *Client) GetTailcatStatus(ctx context.Context) (*TailcatStatus, error) {
	var result TailcatStatus
	if err := c.doJSON(ctx, "/api/tailcat", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UnloadAllModels sends POST /api/models/unload.
func (c *Client) UnloadAllModels(ctx context.Context) error {
	return c.doJSONPost(ctx, "/api/models/unload", nil, nil)
}

// UnloadModel sends POST /api/models/unload/{model}.
func (c *Client) UnloadModel(ctx context.Context, model string) error {
	return c.doJSONPost(ctx, "/api/models/unload/"+model, nil, nil)
}

// CancelInflightRequest sends POST /api/inflight/{id}/cancel.
func (c *Client) CancelInflightRequest(ctx context.Context, id string) error {
	return c.doJSONPost(ctx, "/api/inflight/"+id+"/cancel", nil, nil)
}

// LoadModel initiates loading a model via GET /upstream/{model}.
func (c *Client) LoadModel(ctx context.Context, model string) error {
	url := c.BaseURL + "/upstream/" + model + "/?_=" + fmt.Sprintf("%d", time.Now().UnixMilli())
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("load model %s: %w", model, err)
	}
	for k, vals := range c.Header {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}

	resp, err := c.httpCli.Do(req)
	if err != nil {
		return fmt.Errorf("load model %s: %w", model, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("load model %s: HTTP %d", model, resp.StatusCode)
	}
	return nil
}

// GetCapture fetches a request/response capture by ID.
func (c *Client) GetCapture(ctx context.Context, id int) (*ReqRespCapture, error) {
	var result ReqRespCapture
	if err := c.doJSON(ctx, fmt.Sprintf("/api/captures/%d", id), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// StreamLogs connects to /logs/stream and returns a channel that receives
// raw log lines. The reader runs until ctx is cancelled or the client is
// explicitly closed.
func (c *Client) StreamLogs(ctx context.Context, source string) (<-chan string, error) {
	ch := make(chan string, 256)
	go func() {
		defer close(ch)
		url := c.BaseURL + "/logs/stream"
		if source != "" {
			url += "/" + source
		}

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				time.Sleep(1 * time.Second)
				continue
			}
			req.Header.Set("Accept", "text/event-stream")
			req.Header.Set("Cache-Control", "no-cache")
			for k, vals := range c.Header {
				for _, v := range vals {
					req.Header.Add(k, v)
				}
			}

			resp, err := c.httpCli.Do(req)
			if err != nil {
				time.Sleep(1 * time.Second)
				continue
			}

			buf := make([]byte, 0, 8192)
			for {
				b := make([]byte, 1024)
				n, readErr := resp.Body.Read(b)
				buf = append(buf, b[:n]...)

				for {
					idx := bytes.IndexByte(buf, '\n')
					if idx == -1 {
						break
					}
					line := string(buf[:idx])
					buf = buf[idx+1:]
					line = strings.TrimRight(line, "\r")

					// SSE data lines start with "data:"
					if strings.HasPrefix(line, "data:") {
						data := strings.TrimPrefix(line, "data:")
						if len(data) > 0 {
							select {
							case ch <- data:
							case <-ctx.Done():
							}
						}
					}
				}

				if readErr != nil {
					if readErr == io.EOF {
						break
					}
					select {
					case <-ctx.Done():
						resp.Body.Close()
						return
					default:
					}
				}
			}
			resp.Body.Close()
			time.Sleep(1 * time.Second)
		}
	}()
	return ch, nil
}
