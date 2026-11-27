package tui

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"llama-swap-tui/api"

	"github.com/charmbracelet/x/ansi"
)

// captureDetail is the parsed input prompt and output text of a
// request/response capture, ready to render in the activity detail view.
type captureDetail struct {
	input     string
	output    string
	reasoning string
}

// parseCapture extracts the input prompt and output from a capture,
// mirroring the web UI's CaptureDialog: request JSON messages become the
// input prompt, the response JSON (or reassembled SSE chunks) becomes the
// output. Bodies the parser does not recognise fall back to raw (pretty
// printed when JSON) text rather than failing.
func parseCapture(c api.ReqRespCapture) captureDetail {
	output, reasoning := parseOutput(c)
	return captureDetail{
		input:     parseInputPrompt(decodeBody(c.ReqBody)),
		output:    output,
		reasoning: reasoning,
	}
}

// decodeBody decodes a base64 capture body. The server stores bodies as
// []byte, so JSON carries them base64-encoded; a body that is not valid
// base64 is already plain text and passes through unchanged.
func decodeBody(body string) string {
	if body == "" {
		return ""
	}
	if b, err := base64.StdEncoding.DecodeString(body); err == nil {
		return string(b)
	}
	return body
}

// chatMessage is one entry of a chat completions messages array.
type chatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// parseInputPrompt renders the request body as the input prompt: chat
// messages as "role: content" lines, a completions prompt, or an
// embeddings input. Anything else falls back to the raw body.
func parseInputPrompt(reqBody string) string {
	var req struct {
		Messages []chatMessage `json:"messages"`
		Prompt   any           `json:"prompt"`
		Input    any           `json:"input"`
	}
	if err := json.Unmarshal([]byte(reqBody), &req); err == nil {
		if len(req.Messages) > 0 {
			var lines []string
			for _, msg := range req.Messages {
				if text := messageContent(msg.Content); text != "" {
					lines = append(lines, msg.Role+": "+text)
				}
			}
			if len(lines) > 0 {
				return strings.Join(lines, "\n")
			}
		}
		if text := anyText(req.Prompt); text != "" {
			return text
		}
		if text := anyText(req.Input); text != "" {
			return text
		}
	}
	return prettyOrRaw(reqBody)
}

// parseOutput renders the response body as the output text. Streamed
// responses (text/event-stream) are reassembled from their SSE chunks;
// non-streamed chat responses come from choices[0].message.content,
// completions from choices[0].text. Anything else falls back to the raw
// body.
func parseOutput(c api.ReqRespCapture) (output, reasoning string) {
	body := decodeBody(c.RespBody)
	if isSSE(c.RespHeaders) {
		return parseSSEChat(body)
	}
	var resp struct {
		Choices []struct {
			Message struct {
				Content          any `json:"content"`
				ReasoningContent any `json:"reasoning_content"`
			} `json:"message"`
			Text string `json:"text"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err == nil && len(resp.Choices) > 0 {
		ch := resp.Choices[0]
		if text := messageContent(ch.Message.Content); text != "" {
			return text, anyText(ch.Message.ReasoningContent)
		}
		if ch.Text != "" {
			return ch.Text, ""
		}
	}
	return prettyOrRaw(body), ""
}

// parseSSEChat reassembles streamed chat output: each "data: {...}" line
// carries choices[0].delta.content (and reasoning_content), "[DONE]"
// ends the stream. Unparseable lines are skipped, like the web UI.
func parseSSEChat(body string) (content, reasoning string) {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
					Reasoning        string `json:"reasoning"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil || len(chunk.Choices) == 0 {
			continue
		}
		d := chunk.Choices[0].Delta
		content += d.Content
		reasoning += d.ReasoningContent + d.Reasoning
	}
	return content, reasoning
}

// messageContent renders a chat message's content, which is either a
// plain string or an array of content parts ({type:"text", text:"..."}).
func messageContent(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		var parts []string
		for _, p := range t {
			if m, ok := p.(map[string]any); ok {
				if text, ok := m["text"].(string); ok && text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

// anyText renders a prompt/input field that may be a string, a list of
// strings, or a list of content parts.
func anyText(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		var parts []string
		for _, p := range t {
			switch q := p.(type) {
			case string:
				if q != "" {
					parts = append(parts, q)
				}
			case map[string]any:
				if text, ok := q["text"].(string); ok && text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

// prettyOrRaw pretty-prints JSON bodies and passes anything else through
// unchanged, so an unrecognised body is still readable.
func prettyOrRaw(s string) string {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err == nil {
		if b, err := json.MarshalIndent(v, "", "  "); err == nil {
			return string(b)
		}
	}
	return s
}

// isSSE reports whether the response was a streamed (SSE) body, from its
// Content-Type header.
func isSSE(headers map[string]string) bool {
	return strings.Contains(strings.ToLower(contentType(headers)), "text/event-stream")
}

// contentType reads the Content-Type header without caring about case.
func contentType(headers map[string]string) string {
	for k, v := range headers {
		if strings.EqualFold(k, "Content-Type") {
			return v
		}
	}
	return ""
}

// wordWrap wraps s to the given display width, preserving blank lines
// and each line's leading indentation. Words longer than the width are
// hard-split so nothing is ever silently truncated.
func wordWrap(s string, width int) []string {
	if width < 1 {
		width = 1
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		out = append(out, wrapLine(para, width)...)
	}
	return out
}

func wrapLine(line string, width int) []string {
	if ansi.StringWidth(line) <= width {
		return []string{line}
	}
	indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	rest := strings.TrimLeft(line, " \t")
	if ansi.StringWidth(indent) >= width {
		indent = ""
	}
	indentW := ansi.StringWidth(indent)

	// Split overlong words first so every word fits a line on its own.
	var words []string
	for _, w := range strings.Fields(rest) {
		words = append(words, splitWord(w, width-indentW)...)
	}

	var lines []string
	cur := indent
	curW := indentW
	for _, w := range words {
		ww := ansi.StringWidth(w)
		if curW > indentW && curW+1+ww > width {
			lines = append(lines, cur)
			cur = indent
			curW = indentW
		}
		if curW > indentW {
			cur += " "
			curW++
		}
		cur += w
		curW += ww
	}
	return append(lines, cur)
}

// splitWord hard-splits w into chunks of at most maxW display width.
func splitWord(w string, maxW int) []string {
	if maxW < 1 {
		maxW = 1
	}
	if ansi.StringWidth(w) <= maxW {
		return []string{w}
	}
	var chunks []string
	rest := w
	for ansi.StringWidth(rest) > maxW {
		chunk := ansi.Cut(rest, 0, maxW)
		chunks = append(chunks, chunk)
		rest = ansi.Cut(rest, maxW, ansi.StringWidth(rest))
	}
	if rest != "" {
		chunks = append(chunks, rest)
	}
	return chunks
}
