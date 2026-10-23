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
// second with no network.
const loginWait = 10 * time.Second

// WorkerRefusal refuses a worker while the folder has no login of its own:
// the person logs it in once, in a terminal, since a login waits for a
// browser and rewake never waits for input. `claude auth status` exits 1
// with loggedIn false when there is none (probed on 2.1.302). Under a wrapper
// the wrapper decides the configuration; a status that cannot be read says
// nothing, and the launch goes on.
func (claudeHarness) WorkerRefusal(command string) error {
	if command != "" {
		return nil
	}
	dir, _, err := profileDirPath()
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), loginWait)
	defer cancel()
	status := exec.CommandContext(ctx, "claude", "auth", "status", "--json")
	status.Env = append(os.Environ(), configDirEnv+"="+dir)
	out, err := status.Output()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || !loggedOut(out) {
		return nil
	}
	return fmt.Errorf("a Claude Code worker has a login of its own, and %s has none yet: the person logs it in once, in a terminal, with %s=%s claude auth login", dir, configDirEnv, dir)
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
