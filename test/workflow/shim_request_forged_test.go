package workflow

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"slices"
	"strings"
)

// shimRequest is one command a scenario asks a session to run.
type shimRequest struct {
	Args []string `json:"args"`
	// Env changes the session's environment for this command alone:
	// NAME=value sets a variable, a bare NAME removes it. A space's worker
	// inherits the environment of the command that opens it, and must not
	// take the opener's own shim files.
	Env []string `json:"env,omitempty"`
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

// run runs the request and returns its exit code and what it printed.
func (r shimRequest) run() (int, string) {
	command := exec.Command("rewake", r.Args...)
	if len(r.Env) > 0 {
		command.Env = changedEnv(os.Environ(), r.Env)
	}
	out, err := command.CombinedOutput()
	return exitCode(err), string(out)
}

func changedEnv(env, changes []string) []string {
	for _, change := range changes {
		name, _, set := strings.Cut(change, "=")
		env = slices.DeleteFunc(env, func(kv string) bool { return strings.HasPrefix(kv, name+"=") })
		if set {
			env = append(env, change)
		}
	}
	return env
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
