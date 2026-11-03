package sessiontools

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// enterEnv is adapter.WorktreeEnterEnv, which the core may not import: "1" in
// the run of a worker that may enter a checkout, and rewake worktree enter
// refuses without it.
const enterEnv = "REWAKE_WORKTREE_ENTER"

// A Run is what the wrapper knows of the run a call comes from.
type Run struct {
	// Executable is the rewake the run's room runs, the run's
	// REWAKE_EXECUTABLE.
	Executable string
	Dir        string
	Room       string
	Session    string
	Epoch      string
	Role       string
	// Enter says the run's template lets it enter a checkout.
	Enter bool
}

// A Plan is a call's command, for the agent to run natively: by argv, no
// shell, the run's variables set over the harness's environment, so the
// command answers for this run whatever the harness passes its children.
type Plan struct {
	Argv      []string          `json:"argv"`
	Env       map[string]string `json:"env"`
	TimeoutMs int64             `json:"timeoutMs"`
}

// Plan is the command of a call of the tool named with its arguments in run,
// or why none runs: a tool the run was not offered, or input that cannot make
// one.
func (run Run) Plan(name string, input map[string]any) (Plan, string) {
	tool, ok := run.offered(name)
	if !ok {
		return Plan{}, name + " is not a tool of this session"
	}
	if run.Executable == "" {
		return Plan{}, "this session was not started by a rewake that sets REWAKE_EXECUTABLE"
	}
	args, refused := tool.args(input)
	if refused != "" {
		return Plan{}, refused
	}
	env := map[string]string{}
	for name, value := range map[string]string{
		state.DirEnv:     run.Dir,
		state.RoomEnv:    run.Room,
		state.SessionEnv: run.Session,
		state.EpochEnv:   run.Epoch,
	} {
		if value != "" {
			env[name] = value
		}
	}
	if tool.Name == worktreeEnter.Name {
		env[enterEnv] = "1"
	}
	return Plan{
		Argv:      append([]string{run.Executable}, args...),
		Env:       env,
		TimeoutMs: tool.timeout.Milliseconds(),
	}, ""
}

func (run Run) offered(name string) (Tool, bool) {
	for _, tool := range Offered(run.Role, run.Enter) {
		if tool.Name == name {
			return tool, true
		}
	}
	return Tool{}, false
}

// Ran is what the agent says of a planned command, as its harness reported
// it, unworded.
type Ran struct {
	// ExitCode is nil when the harness names none.
	ExitCode *int   `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	// Killed is a child the harness stopped at the bound or for an abort.
	Killed bool `json:"killed,omitempty"`
	// StdoutCut is an output the harness or the agent's bound cut short.
	StdoutCut bool `json:"stdoutCut,omitempty"`
	// Error is why the command did not run to its end, as the harness threw
	// it: it could not start, or ran past the bound.
	Error string `json:"error,omitempty"`
}

// An Outcome is a tool's result before the harness shapes it.
type Outcome struct {
	Text    string `json:"text"`
	IsError bool   `json:"isError"`
}

// Answer is the result of a call of the tool named: the command's trimmed
// output when it succeeded whole, otherwise an error that says what went
// wrong. A cut answer is an error, not a shorter answer.
func Answer(name string, ran Ran) Outcome {
	tool, _ := Named(name)
	what := tool.what
	if what == "" {
		what = name
	}
	if ran.Error != "" {
		return DidNotEnd(what, ran.Error)
	}
	if ran.Killed {
		return DidNotEnd(what, fmt.Sprintf("stopped after %d ms or by an abort", tool.timeout.Milliseconds()))
	}
	if code := ran.ExitCode; code == nil || *code != 0 {
		said := strings.TrimSpace(ran.Stderr)
		if said == "" {
			said = strings.TrimSpace(ran.Stdout)
		}
		if said == "" {
			said = "no output"
		}
		exit := "null"
		if code != nil {
			exit = fmt.Sprint(*code)
		}
		return Outcome{IsError: true, Text: fmt.Sprintf("%s failed (exit %s): %s", what, exit, said)}
	}
	if ran.StdoutCut {
		return Outcome{IsError: true, Text: what + " answered more than the harness keeps; its answer was cut"}
	}
	return Outcome{Text: strings.TrimSpace(ran.Stdout)}
}

// DidNotEnd is the error of a command that did not run to its end: it could
// not start, ran past its bound or was aborted.
func DidNotEnd(what, why string) Outcome {
	return Outcome{IsError: true, Text: what + " did not run to its end: " + why}
}

// Entered is the path of the checkout rewake worktree enter answered, from
// its JSON.
func Entered(stdout string) (string, error) {
	var entry struct {
		Checkout struct {
			Path string `json:"path"`
		} `json:"checkout"`
	}
	if err := json.Unmarshal([]byte(stdout), &entry); err != nil {
		return "", err
	}
	if entry.Checkout.Path == "" {
		return "", errors.New("no checkout path")
	}
	return entry.Checkout.Path, nil
}

// Digest names a table's content: an agent that holds the table of this
// digest has nothing to register again.
func Digest(tools []Tool) string {
	data, err := json.Marshal(tools)
	if err != nil {
		panic("sessiontools: a table that does not encode: " + err.Error())
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8])
}
