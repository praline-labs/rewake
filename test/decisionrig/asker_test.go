//go:build rewakefixture

package decisionrig

// The asker: this test binary again, started by a half of a session's program
// for one question, which it asks as the command the job names would — the
// product's client, a request written straight to the address, or the person
// classifier — and whose answer it writes to the job's result file.

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/core/authority"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/person"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// askerEnv names the job of an asker; launcherEnv the job of a process that
// starts the asker detached and ends at once, so the asker is adopted.
const (
	askerEnv    = "DECISIONRIG_ASKER"
	launcherEnv = "DECISIONRIG_LAUNCHER"
)

// How an asker asks.
const (
	// modeDecide asks through authority.Decide, the product's client.
	modeDecide = "decide"
	// modeRaw writes the decision's request straight to the address,
	// skipping every check the client makes of the listener.
	modeRaw = "raw"
	// modePerson classifies the asker as the person or not.
	modePerson = "person"
)

// How an asker leaves its parent's tree.
const (
	detachSetsid = "setsid"
	detachDouble = "double"
)

// job is one asker's question and the process it runs as.
type job struct {
	Mode    string             `json:"mode"`
	Address string             `json:"address,omitempty"`
	Run     authority.Run      `json:"run"`
	Ask     mail.DecideRequest `json:"ask"`
	Path    string             `json:"path,omitempty"`
	Root    string             `json:"root,omitempty"`
	// Pause holds the asker at the client's "decision asked" step: checked
	// the wrapper, connected, the question not yet sent.
	Pause string `json:"pause,omitempty"`
	// Set and Unset change the half's environment; Clean starts from none
	// but PATH and HOME.
	Set       []string `json:"set,omitempty"`
	Unset     []string `json:"unset,omitempty"`
	Clean     bool     `json:"clean,omitempty"`
	Detach    string   `json:"detach,omitempty"`
	Namespace bool     `json:"namespace,omitempty"`
	Out       string   `json:"out,omitempty"`
}

// environment is the asker's environment, made from its parent's. No fault
// plan reaches it: its own pause is the job's.
func (j job) environment(parent []string) []string {
	var env []string
	for _, entry := range parent {
		name, _, _ := strings.Cut(entry, "=")
		if name == "REWAKE_FAULT" || name == programEnv || slices.Contains(j.Unset, name) ||
			j.Clean && name != "PATH" && name != "HOME" {
			continue
		}
		env = append(env, entry)
	}
	return append(env, j.Set...)
}

// The kinds of answer an asker reports beside a decision's outcomes.
const (
	kindDecided     = "decided"
	kindUnconfirmed = "unconfirmed"
	kindUnreachable = "unreachable"
	kindError       = "error"
	kindUnstarted   = "unstarted"
)

// result is what an asker answers: a decision made, a refusal by outcome, or
// the client's own refusal of the listener; the person's verdict.
type result struct {
	Kind    string         `json:"kind"`
	Error   string         `json:"error,omitempty"`
	ID      string         `json:"id,omitempty"`
	By      mail.DecidedBy `json:"by"`
	Path    string         `json:"path,omitempty"`
	Verdict string         `json:"verdict,omitempty"`
	// Below is the ancestor that placed a classified process inside a
	// session.
	Below int `json:"below,omitempty"`
	PID   int `json:"pid,omitempty"`
}

func resultOf(err error) result {
	if err != nil {
		return result{Kind: kindError, Error: err.Error()}
	}
	return result{Kind: "done"}
}

func runLauncher(path string) int {
	cmd := exec.Command(os.Args[0])
	var env []string
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, launcherEnv+"=") {
			env = append(env, entry)
		}
	}
	env = append(env, askerEnv+"="+path)
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return 1
	}
	return 0
}

func runAsker(path string) int {
	var j job
	if err := readJSON(path, &j); err != nil {
		return 1
	}
	if j.Pause != "" {
		state.Fault = func(op, step string) error {
			if op == state.OpStep && step == "decision asked" {
				hold(j.Pause)
			}
			return nil
		}
	}
	r := ask(j)
	r.PID = os.Getpid()
	if err := writeJSON(j.Out, r); err != nil {
		return 1
	}
	return 0
}

func ask(j job) result {
	switch j.Mode {
	case modeDecide:
		answer, err := authority.Decide(j.Address, j.Run, j.Ask, j.Path)
		if err != nil {
			return refusal(err)
		}
		return result{Kind: kindDecided, ID: answer.Decision.ID, By: answer.Decision.By, Path: answer.Path}
	case modeRaw:
		return raw(j.Address, map[string]any{"op": "decide", "decide": map[string]any{
			"sender": j.Ask.Sender, "report": j.Ask.Report, "choice": j.Ask.Choice, "expect": j.Ask.Expect,
		}})
	case modePerson:
		err := person.Classify(j.Root)
		var below *person.BelowError
		var unreadable *person.UnreadableError
		switch {
		case err == nil:
			return result{Kind: "done", Verdict: "person"}
		case errors.As(err, &below):
			return result{Kind: "done", Verdict: "below", Below: below.PID, Error: err.Error()}
		case errors.As(err, &unreadable):
			return result{Kind: "done", Verdict: "unreadable", Error: err.Error()}
		}
		return result{Kind: kindError, Error: err.Error()}
	}
	return result{Kind: kindError, Error: "no mode " + j.Mode}
}

func refusal(err error) result {
	var decision *mail.DecisionError
	switch {
	case errors.As(err, &decision):
		return result{Kind: string(decision.Outcome), Error: err.Error(), Path: decision.Path}
	case errors.Is(err, authority.ErrNotConfirmed):
		return result{Kind: kindUnconfirmed, Error: err.Error()}
	case errors.Is(err, authority.ErrUnreachable):
		return result{Kind: kindUnreachable, Error: err.Error()}
	}
	return result{Kind: kindError, Error: err.Error()}
}

// raw writes one request line to the address and reads the answer line, as
// a forger's own client would.
func raw(address string, request any) result {
	conn, err := net.DialTimeout("unix", address, 5*time.Second)
	if err != nil {
		return result{Kind: kindUnreachable, Error: err.Error()}
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(70 * time.Second))
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return result{Kind: kindUnreachable, Error: err.Error()}
	}
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return result{Kind: kindUnreachable, Error: err.Error()}
	}
	var answer struct {
		Error   string `json:"error"`
		Decided *struct {
			ID      string         `json:"id"`
			By      mail.DecidedBy `json:"by"`
			Outcome string         `json:"outcome"`
			Error   string         `json:"error"`
		} `json:"decided"`
	}
	if err := json.Unmarshal(line, &answer); err != nil {
		return result{Kind: kindError, Error: string(line)}
	}
	switch {
	case answer.Error != "":
		return result{Kind: kindError, Error: answer.Error}
	case answer.Decided == nil:
		return result{Kind: "done"}
	case answer.Decided.Outcome != "":
		return result{Kind: answer.Decided.Outcome, Error: answer.Decided.Error}
	}
	return result{Kind: kindDecided, ID: answer.Decided.ID, By: answer.Decided.By}
}

// hold writes held into dir and waits, at most a minute, for go.
func hold(dir string) {
	_ = os.WriteFile(filepath.Join(dir, "held"), nil, 0o600)
	for deadline := time.Now().Add(time.Minute); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if _, err := os.Stat(filepath.Join(dir, "go")); err == nil {
			return
		}
	}
}
