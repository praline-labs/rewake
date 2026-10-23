package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// configDirEnv moves Claude Code's whole global layer — settings, memory,
// skills, login, trust, transcripts — into one directory. A worker's is the
// service folder (the owner's decision, October 23, 2026), so its
// conversations stay out of the person's resume lists and its login out of
// the person's credentials.
const configDirEnv = "CLAUDE_CONFIG_DIR"

// loginWait bounds the status check; on 2.1.302 it answers in a fifth of a
// second with no network. A variable for the test of a status that hangs.
var loginWait = 10 * time.Second

// statusWaitDelay bounds the wait for the status's output after loginWait
// kills it: a claude on PATH that is a wrapper may leave a child holding the
// pipes open, and the open would wait on it (review, October 23, 2026).
const statusWaitDelay = 2 * time.Second

// WorkerNotice tells main when the folder has no login of its own yet: the
// person logs in once, in the worker's pane, where Claude Code starts to its
// prompt with "Not logged in · Run /login" in the footer — it shows no login
// screen of its own — and the login then covers every later worker (the
// owner's decision, October 23, 2026). `claude auth status` exits 1 with
// loggedIn false when there is none (probed on 2.1.302). A task sent before
// the login is delivered, its turn stops at once on the missing login, and
// the task stays unread (probed on 2.1.302). Under a wrapper the wrapper
// decides the configuration; a status that cannot be read, or does not
// answer in time, says nothing.
func (claudeHarness) WorkerNotice(command string) string {
	if command != "" {
		return ""
	}
	dir, _, err := profileDirPath()
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), loginWait)
	defer cancel()
	status := exec.CommandContext(ctx, "claude", "auth", "status", "--json")
	status.Env = append(os.Environ(), configDirEnv+"="+dir)
	status.WaitDelay = statusWaitDelay
	out, err := status.Output()
	var exit *exec.ExitError
	if ctx.Err() != nil || !errors.As(err, &exit) || !loggedOut(out) {
		return ""
	}
	return fmt.Sprintf("The workers' Claude Code is not logged in: %s has no login of its own yet. Tell the person to log in once in this worker's pane with /login; the login is kept in that folder and serves every later Claude Code worker. Until then a task's turn stops at once on the missing login and the task stays unread in the worker's inbox: send tasks after the login, and the worker's next rewake inbox reads one sent before.", dir)
}

// loggedOut reads the status's answer: only an explicit loggedIn false.
func loggedOut(out []byte) bool {
	var status struct {
		LoggedIn *bool `json:"loggedIn"`
	}
	if err := json.NewDecoder(bytes.NewReader(out)).Decode(&status); err != nil {
		return false
	}
	return status.LoggedIn != nil && !*status.LoggedIn
}
