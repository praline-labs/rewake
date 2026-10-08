package workflow

// The half of the shim a directory grant needs: the thread's workspace roots,
// which a start or a resume sets and a turn/start that carries them replaces,
// and the status a thread/read answers with. The real server keeps both — a
// root added by turn/start stays for later turns until another list replaces
// it, and thread/read reports the thread working while a turn runs (seen live
// on 0.155.1 and 0.157.1, docs/research-codex.md#runtime-workspace-roots). A
// fixture that answered every read idle and rootless would fail every grant
// and never let one wait.

import (
	"encoding/json"
	"slices"
	"strings"
)

// shimRoots is the thread's roots and whether a turn is running. Guarded by
// the session's lock.
type shimRoots struct {
	list   []string
	known  bool
	active bool
}

// rootsOf reads runtimeWorkspaceRoots from a request, and whether it was there.
// The request has been checked already, so an unreadable field is absent.
func rootsOf(params json.RawMessage) ([]string, bool) {
	var asked struct {
		Roots *[]string `json:"runtimeWorkspaceRoots"`
	}
	if json.Unmarshal(params, &asked) != nil || asked.Roots == nil {
		return nil, false
	}
	return slices.Clone(*asked.Roots), true
}

// seedRoots takes the roots a start or resume names. Call with the lock held.
func (s *shimSession) seedRoots(params json.RawMessage) {
	if roots, ok := rootsOf(params); ok {
		s.roots.list, s.roots.known = roots, true
	}
}

// takeRoots applies the roots a turn/start carries, and records them against
// the turn and the message: a scenario about a grant reads which delivery
// carried which list. A delivery without the field leaves the list as it was
// and records nothing, because the turn log is also how a session tells it has
// worked a turn (workedATurn), and a line written before the turn runs would
// say so too early. Call with the lock held.
func (s *shimSession) takeRoots(turn string, params json.RawMessage) {
	roots, ok := rootsOf(params)
	if !ok {
		return
	}
	s.roots.list, s.roots.known = roots, true
	s.recordTurnEvent("roots", turn, messageIDOf(params)+" "+strings.Join(roots, ":"))
}

// setActive records the status the thread reports from now on. Call with the
// lock held, beside the thread/status/changed event that says the same.
func (s *shimSession) setActive(active bool) { s.roots.active = active }

// currentStatus is the status a thread object carries.
func (s *shimSession) currentStatus() map[string]any {
	if s.roots.active {
		return activeStatus()
	}
	return idleStatus()
}

// environments is Thread.environments: one local environment with the roots
// the thread holds, the shape the adapter requires before it changes them.
// Absent until a start or resume named roots.
func (s *shimSession) environments() []map[string]any {
	if !s.roots.known {
		return nil
	}
	return []map[string]any{{"environmentId": "local", "cwd": "/work", "runtimeWorkspaceRoots": slices.Clone(s.roots.list)}}
}
