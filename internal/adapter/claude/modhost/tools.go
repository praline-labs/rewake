package modhost

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/praline-labs/rewake/internal/core/sessiontools"
)

// rewake's tools as data (docs/worktree-tools.md): the table goes to the mod
// in the hello's answer and in a poll's when the load holds another, a call is
// planned here and run by the mod, and its raw end is worded here. Nothing of
// a call is kept: a plan or a result refused in a switch's freeze goes to the
// next image, which answers it alike.

// table is the run's tools, by the current load's word on entry, as the mod
// registers them, and the digest that names them.
func (s *Server) table() ([]ToolEntry, string) {
	tools := sessiontools.Offered(s.cfg.Tools.Role, s.entering())
	entries := make([]ToolEntry, 0, len(tools))
	for _, tool := range tools {
		entries = append(entries, ToolEntry{Name: tool.Name, Description: tool.Description, InputSchema: tool.InputSchema})
	}
	return entries, sessiontools.Digest(tools)
}

// entering says the current load's run may enter a checkout of its own.
func (s *Server) entering() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current != nil && s.current.hello.Enter != nil && *s.current.hello.Enter
}

// run is the run as a call's command answers for it.
func (s *Server) run() sessiontools.Run {
	run := s.cfg.Tools
	run.Enter = s.entering()
	return run
}

func (s *Server) toolPlan(w http.ResponseWriter, r *http.Request) {
	var call ToolPlanRequest
	if !s.toolRequest(w, r, &call, MaxBodyBytes, func() string { return call.LoadID }) {
		return
	}
	var input map[string]any
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		s.answer(w, r, ToolPlan{Refused: ptr(call.Tool + "'s arguments are no JSON object")})
		return
	}
	plan, refused := s.run().Plan(call.Tool, input)
	if refused != "" {
		s.answer(w, r, ToolPlan{Refused: &refused})
		return
	}
	env := make([]EnvVar, 0, len(plan.Env))
	for name, value := range plan.Env {
		env = append(env, EnvVar{Name: name, Value: value})
	}
	slices.SortFunc(env, func(a, b EnvVar) int { return strings.Compare(a.Name, b.Name) })
	timeout := int(plan.TimeoutMs)
	s.answer(w, r, ToolPlan{Argv: plan.Argv, Env: env, TimeoutMs: &timeout})
}

func (s *Server) toolResult(w http.ResponseWriter, r *http.Request) {
	var result ToolResultRequest
	if !s.toolRequest(w, r, &result, MaxEndBodyBytes, func() string { return result.LoadID }) {
		return
	}
	ran := sessiontools.Ran{
		ExitCode: result.Ran.ExitCode, Stdout: result.Ran.Stdout, Stderr: result.Ran.Stderr,
		StdoutCut: result.Ran.StdoutCut != nil && *result.Ran.StdoutCut,
	}
	if result.Ran.Error != nil {
		ran.Error = *result.Ran.Error
	}
	outcome := sessiontools.Answer(result.Tool, ran)
	answer := ToolAnswer{Text: outcome.Text, IsError: outcome.IsError}
	if result.Tool == enterTool && !outcome.IsError {
		path, err := sessiontools.Entered(outcome.Text)
		if err != nil {
			failed := sessiontools.DidNotEnd("reading rewake worktree enter's answer", err.Error())
			answer = ToolAnswer{Text: failed.Text, IsError: true}
		} else {
			answer.EnterWorktree = &path
		}
	}
	s.answer(w, r, answer)
}

// enterTool is the tool whose result moves the session into a checkout.
const enterTool = "worktree_enter"

// toolNative words the move into a checkout worktree_enter made, as Claude
// Code answered EnterWorktree. The checkout is the run's from the make on: a
// failed move leaves it claimed, a later call answers the same one, and the
// close releases it.
func (s *Server) toolNative(w http.ResponseWriter, r *http.Request) {
	var native ToolNativeRequest
	if !s.toolRequest(w, r, &native, MaxEndBodyBytes, func() string { return native.LoadID }) {
		return
	}
	s.answer(w, r, entered(native))
}

func entered(native ToolNativeRequest) ToolAnswer {
	if native.Inside {
		return ToolAnswer{Text: native.Made + "\nThis session already works in it."}
	}
	stays := "The worktree at " + native.Path + " is this worker's and stays; call worktree_enter again to enter it."
	moved := native.Moved
	switch {
	case moved == nil:
		return ToolAnswer{IsError: true, Text: sessiontools.DidNotEnd("entering "+native.Path+" ("+stays+")", "no answer").Text}
	case moved.Error != nil:
		return ToolAnswer{IsError: true, Text: sessiontools.DidNotEnd("entering "+native.Path+" ("+stays+")", *moved.Error).Text}
	case moved.Deny != nil:
		return ToolAnswer{IsError: true, Text: "Claude Code refused to enter it: " + *moved.Deny + ". " + stays}
	case moved.IsError != nil && *moved.IsError:
		return ToolAnswer{IsError: true, Text: "Claude Code could not enter it: " + deref(moved.Text) + ". " + stays}
	}
	said := deref(moved.Text)
	if said == "" {
		said = "Entered " + native.Path
	}
	return ToolAnswer{Text: native.Made + "\n" + said}
}

// toolRequest reads a tool request within limit and checks it comes from an
// admitted load, refusing it otherwise.
func (s *Server) toolRequest(w http.ResponseWriter, r *http.Request, into any, limit int64, load func() string) bool {
	if status, err := decode(w, r, into, limit); err != nil {
		refuse(w, status, err.Error())
		return false
	}
	if !s.admittedLoad(load()) {
		refuse(w, http.StatusBadRequest, errNoHello.Error())
		return false
	}
	return true
}

func ptr(text string) *string { return &text }

func deref(text *string) string {
	if text == nil {
		return ""
	}
	return *text
}
