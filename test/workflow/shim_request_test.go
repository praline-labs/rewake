package workflow

import (
	"encoding/json"
	"fmt"
	"os"
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
			var request shimRequest
			raw, err := os.ReadFile(filepath.Join(dir, name))
			if err == nil {
				request, err = parseRequest(raw)
			}
			record := "-1\nunreadable request: " + fmt.Sprint(err)
			base := filepath.Join(dir, strings.TrimSuffix(name, ".args"))
			if err == nil {
				code, out := request.run(base)
				record = strconv.Itoa(code) + "\n" + out
			}
			target := base + ".out"
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

// staysUp is the launch switch for a session asked nothing that still has to
// outlive the ordinary ceiling: its request directory is never written to, and
// serving it is what keeps the session up for requestLifetime.
func staysUp(iso *Isolation, label string) string { return newRequests(iso, label).env() }

// env is the switch the session is launched with.
func (r *requests) env() string { return shimRequestDir + "=" + r.dir }

// ask runs `rewake <args>` in the session. False when the session did not
// answer in time; the output then says what is known.
func (r *requests) ask(c *Case, args ...string) (int, string, bool) {
	encoded, _ := json.Marshal(args)
	return r.send(c, encoded, strings.Join(args, " "))
}

// askWith runs a request with more than arguments: variables over the
// session's own, or a process that leaves the session's tree first.
func (r *requests) askWith(c *Case, request shimRequest) (int, string, bool) {
	encoded, _ := json.Marshal(request)
	return r.send(c, encoded, strings.Join(request.Args, " "))
}

func (r *requests) send(c *Case, encoded []byte, shown string) (int, string, bool) {
	if err := os.MkdirAll(r.dir, 0o700); err != nil {
		return -1, err.Error(), false
	}
	r.next++
	base := filepath.Join(r.dir, strconv.Itoa(r.next))
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
		return -1, "the session never ran rewake " + shown, false
	}
	first, rest, _ := strings.Cut(string(raw), "\n")
	code, err := strconv.Atoi(first)
	if err != nil {
		return -1, string(raw), false
	}
	return code, rest, true
}
