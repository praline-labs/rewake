package workflow

// What the server keeps of a thread between runs: the roots its settings
// were last saved with. A cold thread/resume without the roots field restores
// them when the grant was given on an established history, and a grant on the
// thread's first turn was lost (seen live on 0.155.1 and 0.157.1,
// docs/research-codex.md#runtime-workspace-roots). The fixture saves the
// roots a start names and those of every completed turn after the first, and
// a resume that names none takes them back. Off unless a case names the file
// the thread is kept in, so a case that never resumes keeps its roots in
// memory alone, as before.

import (
	"encoding/json"
	"os"
	"slices"
)

// shimThreadStore is the file a case keeps its thread's saved settings in.
const shimThreadStore = "RW_SHIM_THREAD_STORE"

// savedThread is what the server kept: the roots, and how many turns the
// thread completed.
type savedThread struct {
	Roots []string `json:"roots"`
	Turns int      `json:"turns"`
}

func loadSavedThread() (savedThread, bool) {
	path := os.Getenv(shimThreadStore)
	if path == "" {
		return savedThread{}, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return savedThread{}, false
	}
	var saved savedThread
	if json.Unmarshal(raw, &saved) != nil {
		return savedThread{}, false
	}
	return saved, true
}

func storeSavedThread(saved savedThread) {
	path := os.Getenv(shimThreadStore)
	if path == "" {
		return
	}
	if raw, err := json.Marshal(saved); err == nil {
		_ = os.WriteFile(path, raw, 0o600)
	}
}

// saveStarted keeps the roots a new thread was started with. Call with the
// lock held.
func (s *shimSession) saveStarted() {
	if s.roots.known {
		storeSavedThread(savedThread{Roots: slices.Clone(s.roots.list)})
	}
}

// restoreSaved gives a thread resumed without roots the ones it was saved
// with. Call with the lock held, after the roots the resume named were taken.
func (s *shimSession) restoreSaved(params json.RawMessage) {
	if _, named := rootsOf(params); named {
		return
	}
	if saved, ok := loadSavedThread(); ok {
		s.roots.list, s.roots.known = slices.Clone(saved.Roots), true
	}
}

// saveCompleted counts a turn that completed, and keeps the roots it ran with
// once the thread has a history; a failed turn saves nothing. Call with the
// lock held.
func (s *shimSession) saveCompleted() {
	saved, ok := loadSavedThread()
	if !ok || turnFailed() {
		return
	}
	saved.Turns++
	if saved.Turns > 1 {
		saved.Roots = slices.Clone(s.roots.list)
	}
	storeSavedThread(saved)
}
