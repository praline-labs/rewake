package workflow

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// shimRequest is one command a scenario asks a session to run. Env and
// Detach let a scenario play a worker that forges main: its variables over
// the session's own, and a process that leaves the session's tree before it
// runs rewake.
type shimRequest struct {
	Args   []string `json:"args"`
	Env    []string `json:"env,omitempty"`
	Detach bool     `json:"detach,omitempty"`
}

// parseRequest reads a request in either form: a plain list of arguments, or
// the object above.
func parseRequest(raw []byte) (shimRequest, error) {
	var args []string
	if json.Unmarshal(raw, &args) == nil {
		return shimRequest{Args: args}, nil
	}
	var request shimRequest
	err := json.Unmarshal(raw, &request)
	return request, err
}

// detachWait bounds how long a detached command may take: it is one rewake
// call, which answers in well under a second.
const detachWait = 15 * time.Second

// run runs the request and returns its exit code and what it printed. A
// detached one goes through setsid -f, which forks: the shell it starts is
// orphaned at once and adopted outside this session's tree, and writes its
// own parent beside what rewake printed, so a finding can show where it ran.
func (r shimRequest) run(base string) (int, string) {
	env := append(os.Environ(), r.Env...)
	if !r.Detach {
		command := exec.Command("rewake", r.Args...)
		command.Env = env
		out, err := command.CombinedOutput()
		return exitCode(err), string(out)
	}
	script := `sleep 0.2; grep PPid /proc/$$/status >"$0.log"; rewake "$@" >>"$0.log" 2>&1; echo $? >"$0.code.tmp"; mv "$0.code.tmp" "$0.code"`
	command := exec.Command("setsid", append([]string{"-f", "sh", "-c", script, base}, r.Args...)...)
	command.Env = env
	if out, err := command.CombinedOutput(); err != nil {
		return -1, "setsid: " + err.Error() + ": " + string(out)
	}
	deadline := time.Now().Add(detachWait)
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(base + ".code"); err == nil {
			out, _ := os.ReadFile(base + ".log")
			code, err := strconv.Atoi(strings.TrimSpace(string(raw)))
			if err != nil {
				code = -1
			}
			return code, string(out)
		}
		time.Sleep(50 * time.Millisecond)
	}
	return -1, "the detached command did not finish"
}

func exitCode(err error) int {
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		return exit.ExitCode()
	case err != nil:
		return -1
	}
	return 0
}
