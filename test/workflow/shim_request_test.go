package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// shimRequestDir makes a session run rewake commands a scenario asks for, at a
// moment the scenario picks: the scenario writes <n>.args, a JSON list of
// arguments, and the session runs `rewake <args>` with its own environment and
// writes <n>.out — the exit code on the first line, then what the command
// printed. What only the session's own run can do, such as sending as it or
// listing what it waits on, is then done by the session, not by the test.
const shimRequestDir = "RW_SHIM_REQUEST_DIR"

// requestLifetime is how long a session serving requests stays up unasked, on
// either column: past the ordinary ceiling, which a steered scenario run under a
// slower control outlives, taking its sessions down in the middle of it.
const requestLifetime = 85 * time.Second

// serveRequests runs in the fixture beside everything else, for as long as the
// session lives.
func serveRequests() {
	dir := os.Getenv(shimRequestDir)
	if dir == "" {
		return
	}
	if err := insideACase(); err != nil {
		fmt.Fprintf(os.Stderr, "shim: %v\n", err)
		return
	}
	served := map[string]bool{}
	for {
		entries, _ := os.ReadDir(dir)
		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasSuffix(name, ".args") || served[name] {
				continue
			}
			served[name] = true
			var args []string
			raw, err := os.ReadFile(filepath.Join(dir, name))
			if err == nil {
				err = json.Unmarshal(raw, &args)
			}
			record := "-1\nunreadable request: " + fmt.Sprint(err)
			if err == nil {
				out, err := exec.Command("rewake", args...).CombinedOutput()
				code := 0
				var exit *exec.ExitError
				if errors.As(err, &exit) {
					code = exit.ExitCode()
				} else if err != nil {
					code = -1
				}
				record = strconv.Itoa(code) + "\n" + string(out)
			}
			target := filepath.Join(dir, strings.TrimSuffix(name, ".args")+".out")
			_ = os.WriteFile(target+".tmp", []byte(record), 0o600)
			_ = os.Rename(target+".tmp", target)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// requests is the scenario's side: it asks one session to run a command and
// waits for what the command printed.
type requests struct {
	dir  string
	next int
}

func newRequests(iso *Isolation, label string) *requests {
	return &requests{dir: filepath.Join(iso.Home, label+".requests")}
}

// env is the switch the session is launched with.
func (r *requests) env() string { return shimRequestDir + "=" + r.dir }

// ask runs `rewake <args>` in the session. False when the session did not
// answer in time; the output then says what is known.
func (r *requests) ask(c *Case, args ...string) (int, string, bool) {
	if err := os.MkdirAll(r.dir, 0o700); err != nil {
		return -1, err.Error(), false
	}
	r.next++
	base := filepath.Join(r.dir, strconv.Itoa(r.next))
	encoded, _ := json.Marshal(args)
	// Written aside and renamed, so the session never reads half a request.
	if err := os.WriteFile(base+".tmp", encoded, 0o600); err != nil {
		return -1, err.Error(), false
	}
	if err := os.Rename(base+".tmp", base+".args"); err != nil {
		return -1, err.Error(), false
	}
	var raw []byte
	if !waitFor(c, 20*time.Second, func() bool {
		var err error
		raw, err = os.ReadFile(base + ".out")
		return err == nil
	}) {
		return -1, "the session never ran rewake " + strings.Join(args, " "), false
	}
	first, rest, _ := strings.Cut(string(raw), "\n")
	code, err := strconv.Atoi(first)
	if err != nil {
		return -1, string(raw), false
	}
	return code, rest, true
}
