package claude

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
)

// marksOf plays the collector's side: who interrupted, until told.
type marksOf struct {
	mu   sync.Mutex
	by   string
	told []uint64
}

func (m *marksOf) Interrupter() (string, uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.by, 7
}

func (m *marksOf) Told(mark uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.by, m.told = "", append(m.told, mark)
}

const interruptLine = "lead interrupted your previous turn with rewake interrupt."

func summaryOf(line map[string]any) string {
	message, _ := line["message"].(map[string]any)
	content, _ := message["content"].(string)
	return between(content, "<summary>", "</summary>")
}

// between is the text between two markers, "" when either is missing.
func between(text, opening, closing string) string {
	_, rest, found := strings.Cut(text, opening)
	inner, _, closed := strings.Cut(rest, closing)
	if !found || !closed {
		return ""
	}
	return inner
}

// After a main's interrupt the next notice says so as its last line, once;
// a notice that could not be written does not use it up.
func TestTheNextNoticeSaysAMainInterrupted(t *testing.T) {
	dir := t.TempDir()
	marks := &marksOf{by: "lead"}

	gone := newLane("", false, nil, marks)
	missing := registry.Session{Socket: filepath.Join(dir, "missing.sock")}
	if result := gone.Deliver(context.Background(), missing, inbox.Message{ID: "m0", Kind: inbox.Task, Text: "work"}); result.State == inbox.Delivered {
		t.Fatalf("a missing socket delivered: %+v", result)
	}
	if len(marks.told) != 0 {
		t.Fatal("an undelivered notice used the line up")
	}

	r := startReceiver(t, dir)
	l := startLane(t, dir, nil)
	l.marks = marks
	if result := deliver(l, r, "m1"); result.State != inbox.Delivered {
		t.Fatalf("got %+v", result)
	}
	summary := summaryOf(<-r.lines)
	if !strings.HasPrefix(summary, "Rewake: ") || !strings.HasSuffix(summary, "\n"+interruptLine) {
		t.Fatalf("the first notice after the interrupt reads %q", summary)
	}
	if len(marks.told) != 1 || marks.told[0] != 7 {
		t.Fatalf("told %v", marks.told)
	}
	if result := deliver(l, r, "m2"); result.State != inbox.Delivered {
		t.Fatalf("got %+v", result)
	}
	if summary := summaryOf(<-r.lines); strings.Contains(summary, "interrupted") {
		t.Fatalf("the line came twice: %q", summary)
	}
}
