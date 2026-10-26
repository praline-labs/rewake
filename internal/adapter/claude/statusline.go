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
	"strconv"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/adapter/claude/modhost"
)

// The status line of a worker (docs/claude.md#the-cache-ttl): Claude
// Code runs `rewake statusline` after each response and on a timer, with its
// own JSON on stdin. Its prompt_cache is the only place the cache's lifetime is
// told — the mod cannot read it — and its effort.level the only place an
// /effort shows before the next request (docs/research.md, the status line),
// so the command forwards both, and the model's own name (model.display_name, which follows /model), to the run's host over the mod socket and
// prints the person's own status line, if the workers' settings.json names
// one, in its place.

// statusLineRefresh is how often Claude Code runs the command beside its own
// events; the expiry it reports is absolute, so a slow timer loses nothing.
const statusLineRefresh = 30

// The two deadlines are separate and the two jobs run side by side: the
// telemetry is optional and must not spend the person's line's time, and the
// pane waits for the line's text. Variables so a test can shorten them.
var (
	statusLineTimeout = 5 * time.Second
	postTimeout       = 2 * time.Second
)

// statusLineGuardEnv marks the environment of the person's own command: a
// `rewake statusline` that finds it is that command calling us again, and
// prints and posts nothing, so a person's line that is ours cannot recurse.
const statusLineGuardEnv = "REWAKE_STATUSLINE_PASSTHROUGH"

var _ adapter.StatusLiner = claudeHarness{}

type promptCache struct {
	Warm      bool    `json:"warm"`
	TTL       *string `json:"ttl"`
	ExpiresAt *int64  `json:"expires_at"`
}

// StatusLine posts the input's prompt_cache, effort and model.display_name to the run's host and
// prints the person's own line. Either failing leaves the other alone.
func (claudeHarness) StatusLine(ctx context.Context, input []byte, out io.Writer) {
	if os.Getenv(statusLineGuardEnv) != "" {
		return
	}
	var seen struct {
		PromptCache *promptCache `json:"prompt_cache"`
		Effort      struct {
			Level any `json:"level"`
		} `json:"effort"`
		Model struct {
			DisplayName string `json:"display_name"`
		} `json:"model"`
	}
	posted := make(chan struct{})
	go func() {
		defer close(posted)
		if json.Unmarshal(input, &seen) != nil {
			return
		}
		effort := effortText(seen.Effort.Level)
		if seen.PromptCache != nil || effort != "" || seen.Model.DisplayName != "" {
			post, cancel := context.WithTimeout(ctx, postTimeout)
			defer cancel()
			postLine(post, os.Getenv(modSocketEnv), seen.PromptCache, effort, seen.Model.DisplayName)
		}
	}()
	line, cancel := context.WithTimeout(ctx, statusLineTimeout)
	defer cancel()
	personal(line, input, out)
	<-posted
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

func postLine(ctx context.Context, socket string, cache *promptCache, effort, modelName string) {
	if socket == "" {
		return
	}
	var body modhost.StatusLineRequest
	if cache != nil {
		body.Warm, body.Ttl = &cache.Warm, cache.TTL
		if cache.ExpiresAt != nil {
			at := int(*cache.ExpiresAt)
			body.ExpiresAt = &at
		}
	}
	if effort != "" {
		body.Effort = &effort
	}
	if modelName != "" {
		body.ModelName = &modelName
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
	// The command runs in a process group of its own with its output in a
	// pipe this process reads: at the deadline the whole group is killed and
	// the pipe closed, so nothing a child still holds keeps the harness's
	// output open, and only what came in time is printed.
	var line bytes.Buffer
	command := exec.CommandContext(ctx, "sh", "-c", settings.StatusLine.Command)
	command.Stdin = bytes.NewReader(input)
	command.Stdout = &line
	command.Env = append(os.Environ(), statusLineGuardEnv+"=1")
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error { return syscall.Kill(-command.Process.Pid, syscall.SIGKILL) }
	command.WaitDelay = 500 * time.Millisecond
	_ = command.Run()
	_, _ = out.Write(line.Bytes())
}
