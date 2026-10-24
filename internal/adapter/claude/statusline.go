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
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/adapter/claude/modhost"
)

// The status line of a worker (docs/claude.md#the-cache-ttl): Claude
// Code runs `rewake statusline` after each response and on a timer, with its
// own JSON on stdin. Its prompt_cache is the only place the cache's lifetime is
// told — the mod cannot read it — so the command forwards it to the run's
// host over the mod socket and prints the person's own status line, if the
// workers' settings.json names one, in its place.

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

// StatusLine posts the input's prompt_cache to the run's host and prints the
// person's own line. Either failing leaves the other alone.
func (claudeHarness) StatusLine(ctx context.Context, input []byte, out io.Writer) {
	if os.Getenv(statusLineGuardEnv) != "" {
		return
	}
	var seen struct {
		PromptCache *promptCache `json:"prompt_cache"`
	}
	posted := make(chan struct{})
	go func() {
		defer close(posted)
		if json.Unmarshal(input, &seen) == nil && seen.PromptCache != nil {
			post, cancel := context.WithTimeout(ctx, postTimeout)
			defer cancel()
			postCache(post, os.Getenv(modSocketEnv), *seen.PromptCache)
		}
	}()
	line, cancel := context.WithTimeout(ctx, statusLineTimeout)
	defer cancel()
	personal(line, input, out)
	<-posted
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
