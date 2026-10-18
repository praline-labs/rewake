package workflow

import (
	"encoding/json"
	"errors"
	"os/exec"
)

// shimRequest is one command a scenario asks a session to run.
type shimRequest struct {
	Args []string `json:"args"`
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
	out, err := exec.Command("rewake", r.Args...).CombinedOutput()
	return exitCode(err), string(out)
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
