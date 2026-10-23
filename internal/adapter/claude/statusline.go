package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/adapter/claude/modhost"
)

// The status line of a worker (docs/v2/design-claude.md#the-cache-ttl): Claude
// Code runs `rewake statusline` after each response and on a timer, with its
// own JSON on stdin. Its prompt_cache is the only place the cache's lifetime is
// told — the mod cannot read it — so the command forwards it to the run's
// host over the mod socket and prints the person's own status line, if the
// workers' settings.json names one, in its place.

// statusLineRefresh is how often Claude Code runs the command beside its own
// events; the expiry it reports is absolute, so a slow timer loses nothing.
const statusLineRefresh = 30

// statusLineTimeout bounds the whole command: a status line that stalls
// stalls nothing else, but the pane waits for its text.
const statusLineTimeout = 5 * time.Second

var _ adapter.StatusLiner = claudeHarness{}

type promptCache struct {
	Warm      bool    `json:"warm"`
	TTL       *string `json:"ttl"`
	ExpiresAt *int64  `json:"expires_at"`
}

// StatusLine posts the input's prompt_cache to the run's host and prints the
// person's own line. Either failing leaves the other alone.
func (claudeHarness) StatusLine(ctx context.Context, input []byte, out io.Writer) {
	ctx, cancel := context.WithTimeout(ctx, statusLineTimeout)
	defer cancel()
	var seen struct {
		PromptCache *promptCache `json:"prompt_cache"`
	}
	if json.Unmarshal(input, &seen) == nil && seen.PromptCache != nil {
		postCache(ctx, os.Getenv(modSocketEnv), *seen.PromptCache)
	}
	personal(ctx, input, out)
}

func postCache(ctx context.Context, socket string, cache promptCache) {
	if socket == "" {
		return
	}
	body := modhost.StatusLineRequest{Warm: cache.Warm, Ttl: cache.TTL}
	if cache.ExpiresAt != nil {
		at := int(*cache.ExpiresAt)
		body.ExpiresAt = &at
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

// personal runs the status line the workers' settings.json names, with the
// same input, and copies its output: the overlay's own statusLine shadows it,
// and the person's line should not be lost to the cache's.
func personal(ctx context.Context, input []byte, out io.Writer) {
	folder := os.Getenv("CLAUDE_CONFIG_DIR")
	if folder == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(folder, "settings.json"))
	if err != nil {
		return
	}
	var settings struct {
		StatusLine struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		} `json:"statusLine"`
	}
	if json.Unmarshal(data, &settings) != nil || settings.StatusLine.Type != "command" || settings.StatusLine.Command == "" {
		return
	}
	command := exec.CommandContext(ctx, "sh", "-c", settings.StatusLine.Command)
	command.Stdin = bytes.NewReader(input)
	command.Stdout = out
	command.WaitDelay = time.Second
	_ = command.Run()
}
