package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/praline-labs/rewake/internal/adapter/claude/modhost"
)

// The status line's contract (docs/claude.md#the-cache-ttl): the command a
// run's --settings installs, and what `rewake statusline` takes from Claude
// Code's JSON on stdin and tells the run's host. Running the line is
// statusline.go.

// statusLineRefresh is how often Claude Code runs the command beside its own
// events; the expiry it reports is absolute, so a slow timer loses nothing.
const statusLineRefresh = 30

// statusLineSetting is the run's statusLine: rewake's command, which prints
// nothing, so the person's own line, which it shadows, is off in the run.
// Claude Code merges the key field by field, so the person's padding stands
// (docs/research.md, "How --settings meets a configured status line").
func statusLineSetting(executable string) map[string]any {
	return map[string]any{
		"type":            "command",
		"command":         shellQuote(executable) + " statusline",
		"refreshInterval": statusLineRefresh,
	}
}

// shellQuote makes a path one word of the status line's shell command.
func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

type promptCache struct {
	Warm      bool    `json:"warm"`
	TTL       *string `json:"ttl"`
	ExpiresAt *int64  `json:"expires_at"`
}

// lineInput is what rewake reads of Claude Code's status line input: the
// cache's lifetime, the effort, the model's name and whether it thinks, which
// go to the host.
type lineInput struct {
	PromptCache *promptCache `json:"prompt_cache"`
	Effort      struct {
		Level any `json:"level"`
	} `json:"effort"`
	Model struct {
		DisplayName string `json:"display_name"`
	} `json:"model"`
	Thinking struct {
		Enabled *bool `json:"enabled"`
	} `json:"thinking"`
}

// parseLineInput reads the input; one that does not parse tells nothing.
func parseLineInput(input []byte) (lineInput, bool) {
	var seen lineInput
	return seen, json.Unmarshal(input, &seen) == nil
}

// body is what the host is told; empty when the input told nothing.
func (in lineInput) body() modhost.StatusLineRequest {
	var body modhost.StatusLineRequest
	if cache := in.PromptCache; cache != nil {
		body.Warm, body.Ttl = &cache.Warm, cache.TTL
		if cache.ExpiresAt != nil {
			at := int(*cache.ExpiresAt)
			body.ExpiresAt = &at
		}
	}
	if effort := effortText(in.Effort.Level); effort != "" {
		body.Effort = &effort
	}
	if name := in.Model.DisplayName; name != "" {
		body.ModelName = &name
	}
	body.Thinking = in.Thinking.Enabled
	return body
}

// effortText is an effort level as the mod sends a request's: a level's
// name, or an integer budget as its decimal text; empty for none.
func effortText(level any) string {
	switch level := level.(type) {
	case string:
		return level
	case float64:
		return strconv.FormatFloat(level, 'f', -1, 64)
	}
	return ""
}

func postLine(ctx context.Context, socket string, body modhost.StatusLineRequest) {
	if socket == "" {
		return
	}
	data, err := json.Marshal(body)
	if err != nil {
		return
	}
	client := http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		},
		DisableKeepAlives: true,
	}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://rewake"+modhost.PathStatusLine, bytes.NewReader(data))
	if err != nil {
		return
	}
	request.Header.Set("Content-Type", "application/json")
	if response, err := client.Do(request); err == nil {
		_ = response.Body.Close()
	}
}
