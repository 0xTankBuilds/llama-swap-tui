package tui

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"llama-swap-tui/api"
)

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestParseCaptureChatPromptAndOutput(t *testing.T) {
	cap := api.ReqRespCapture{
		ReqPath: "/v1/chat/completions",
		ReqBody: b64(`{"model":"m","messages":[{"role":"system","content":"Be helpful."},{"role":"user","content":"Hello"}]}`),
		RespHeaders: map[string]string{
			"Content-Type": "application/json",
		},
		RespBody: b64(`{"choices":[{"message":{"role":"assistant","content":"Hi there"},"finish_reason":"stop"}]}`),
	}
	d := parseCapture(cap)
	if want := "system: Be helpful.\nuser: Hello"; d.input != want {
		t.Errorf("input = %q, want %q", d.input, want)
	}
	if d.output != "Hi there" {
		t.Errorf("output = %q, want %q", d.output, "Hi there")
	}
	if d.reasoning != "" {
		t.Errorf("reasoning = %q, want empty", d.reasoning)
	}
}

func TestParseCaptureMessageContentParts(t *testing.T) {
	cap := api.ReqRespCapture{
		ReqBody:  b64(`{"messages":[{"role":"user","content":[{"type":"text","text":"look at this"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AAA"}}]}]}`),
		RespBody: b64(`{"choices":[{"message":{"role":"assistant","content":[{"type":"text","text":"I see it"}]}}]}`),
	}
	d := parseCapture(cap)
	if d.input != "user: look at this" {
		t.Errorf("input = %q, want %q", d.input, "user: look at this")
	}
	if d.output != "I see it" {
		t.Errorf("output = %q, want %q", d.output, "I see it")
	}
}

func TestParseCaptureCompletionsPromptAndText(t *testing.T) {
	cap := api.ReqRespCapture{
		ReqBody:  b64(`{"model":"m","prompt":"Once upon a time"}`),
		RespBody: b64(`{"choices":[{"text":"there was a dragon","finish_reason":"stop"}]}`),
	}
	d := parseCapture(cap)
	if d.input != "Once upon a time" {
		t.Errorf("input = %q, want %q", d.input, "Once upon a time")
	}
	if d.output != "there was a dragon" {
		t.Errorf("output = %q, want %q", d.output, "there was a dragon")
	}
}

func TestParseCaptureSSEReassembly(t *testing.T) {
	sse := "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n" +
		"data: [DONE]\n"
	cap := api.ReqRespCapture{
		ReqBody:     b64(`{"messages":[{"role":"user","content":"Say hi"}]}`),
		RespHeaders: map[string]string{"Content-Type": "text/event-stream"},
		RespBody:    b64(sse),
	}
	d := parseCapture(cap)
	if d.output != "Hello world" {
		t.Errorf("output = %q, want %q", d.output, "Hello world")
	}
}

func TestParseCaptureSSECompletionsText(t *testing.T) {
	sse := "data: {\"choices\":[{\"text\":\"hello\"}]}\n" +
		"data: {\"choices\":[{\"text\":\" world\"}]}\n" +
		"data: [DONE]\n"
	cap := api.ReqRespCapture{
		RespHeaders: map[string]string{"Content-Type": "text/event-stream"},
		RespBody:    b64(sse),
	}
	d := parseCapture(cap)
	if d.output != "hello world" {
		t.Errorf("output = %q, want %q", d.output, "hello world")
	}
}

func TestParseCaptureSSEUnrecognizedFallback(t *testing.T) {
	sse := "data: {\"foo\":\"bar\"}\n" +
		"data: [DONE]\n"
	cap := api.ReqRespCapture{
		RespHeaders: map[string]string{"Content-Type": "text/event-stream"},
		RespBody:    b64(sse),
	}
	d := parseCapture(cap)
	if !strings.Contains(d.output, `"foo"`) {
		t.Errorf("output = %q, want the raw SSE body", d.output)
	}
}

func TestParseCaptureSSEReasoning(t *testing.T) {
	sse := "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"thinking\",\"content\":\"answer\"}}]}\n" +
		"data: not-json\n" +
		"data: {\"choices\":[{\"delta\":{\"reasoning_content\":\" more\"}}]}\n" +
		"data: [DONE]\n"
	cap := api.ReqRespCapture{
		RespHeaders: map[string]string{"content-type": "text/event-stream"},
		RespBody:    b64(sse),
	}
	d := parseCapture(cap)
	if d.output != "answer" {
		t.Errorf("output = %q, want %q", d.output, "answer")
	}
	if d.reasoning != "thinking more" {
		t.Errorf("reasoning = %q, want %q", d.reasoning, "thinking more")
	}
}

func TestParseCaptureNonStreamedReasoning(t *testing.T) {
	cap := api.ReqRespCapture{
		RespBody: b64(`{"choices":[{"message":{"role":"assistant","content":"done","reasoning_content":"I thought about it"}}]}`),
	}
	d := parseCapture(cap)
	if d.output != "done" {
		t.Errorf("output = %q, want %q", d.output, "done")
	}
	if d.reasoning != "I thought about it" {
		t.Errorf("reasoning = %q, want %q", d.reasoning, "I thought about it")
	}
}

func TestParseCaptureNonChatFallback(t *testing.T) {
	cap := api.ReqRespCapture{
		ReqBody:  b64(`{"input":"embed this","model":"m"}`),
		RespBody: b64(`{"data":[{"embedding":[0.1,0.2],"index":0}]}`),
	}
	d := parseCapture(cap)
	if d.input != "embed this" {
		t.Errorf("input = %q, want %q", d.input, "embed this")
	}
	if !strings.Contains(d.output, `"embedding"`) {
		t.Errorf("output = %q, want pretty JSON containing embedding", d.output)
	}
}

func TestParseCaptureInvalidBase64Passthrough(t *testing.T) {
	cap := api.ReqRespCapture{
		ReqBody:  "not base64 at all!!",
		RespBody: "plain text response",
	}
	d := parseCapture(cap)
	if d.input != "not base64 at all!!" {
		t.Errorf("input = %q, want raw body", d.input)
	}
	if d.output != "plain text response" {
		t.Errorf("output = %q, want raw body", d.output)
	}
}

func TestWordWrap(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		width int
		want  []string
	}{
		{
			name:  "short line untouched",
			in:    "hello world",
			width: 20,
			want:  []string{"hello world"},
		},
		{
			name:  "wraps at word boundary",
			in:    "smoke test prompt here",
			width: 10,
			want:  []string{"smoke test", "prompt", "here"},
		},
		{
			name:  "preserves blank lines",
			in:    "first\n\nthird",
			width: 20,
			want:  []string{"first", "", "third"},
		},
		{
			name:  "preserves indentation",
			in:    "  indented line that keeps wrapping along",
			width: 20,
			want:  []string{"  indented line that", "  keeps wrapping", "  along"},
		},
		{
			name:  "hard splits overlong word",
			in:    "supercalifragilistic",
			width: 10,
			want:  []string{"supercalif", "ragilistic"},
		},
		{
			name:  "wide grapheme on a one-cell budget",
			in:    "         界",
			width: 10,
			want:  []string{"         界"},
		},
		{
			name:  "wide grapheme splits at grapheme boundary",
			in:    "ab界cd",
			width: 3,
			want:  []string{"ab", "界c", "d"},
		},
		{
			name:  "ZWJ emoji stays one grapheme",
			in:    "a👨‍👩‍👧b",
			width: 2,
			want:  []string{"a", "👨‍👩‍👧", "b"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := wordWrap(tc.in, tc.width)
			if len(got) != len(tc.want) {
				t.Fatalf("wordWrap(%q, %d) = %q, want %q", tc.in, tc.width, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("line %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestWordWrapWideGraphemeTerminates is the regression test for the
// splitWord infinite loop: a two-cell grapheme on a one-cell budget
// used to hang because ansi.Cut returned "" and the remainder never
// shrank. The timeout turns a future regression into a failure
// instead of hanging the suite.
func TestWordWrapWideGraphemeTerminates(t *testing.T) {
	done := make(chan []string, 1)
	go func() { done <- wordWrap("         界", 10) }()
	select {
	case got := <-done:
		if len(got) != 1 || got[0] != "         界" {
			t.Errorf("wordWrap(%q, 10) = %q, want the line unchanged (overflow, never truncated)", "         界", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("wordWrap hung on a wide grapheme with a one-cell budget")
	}
}
