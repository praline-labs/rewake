package inbox

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// letterWrites counts the letters written through the seam, by id, in any
// stage of any mailbox, and keeps every write's path and every journal state
// written. It fails the failOn-th write of failAt, as a writer stopped there,
// and from the killAt-th write on fails every change, as a process killed
// there.
type letterWrites struct {
	fileAccess
	mu     sync.Mutex
	writes map[string]int
	failAt string
	failOn int
	seen   int
	killAt int
	log    []string
	// journals is every journal state written, in order: the done form
	// keeps no outcome, so what was recorded is read here.
	journals []TurnJournal
}

func countLetters(dir string) *letterWrites {
	counter := &letterWrites{fileAccess: osAccess{}, writes: map[string]int{}}
	testAccess.Store(dir, passAccess{live: counter, plan: counter})
	return counter
}

func (c *letterWrites) WriteFile(path string, raw []byte) error {
	if err := c.note(path, raw); err != nil {
		return err
	}
	return c.fileAccess.WriteFile(path, raw)
}

func (c *letterWrites) Rename(from, to string) error {
	if c.killed() {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: syscall.EIO}
	}
	return c.fileAccess.Rename(from, to)
}

func (c *letterWrites) Remove(path string) error {
	if c.killed() {
		return &os.PathError{Op: "remove", Path: path, Err: syscall.EIO}
	}
	return c.fileAccess.Remove(path)
}

func (c *letterWrites) killed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.killAt > 0 && len(c.log) >= c.killAt
}

func (c *letterWrites) note(path string, raw []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.log = append(c.log, path)
	broken := &os.PathError{Op: "write", Path: path, Err: syscall.EIO}
	if c.killAt > 0 && len(c.log) >= c.killAt {
		return broken
	}
	if path == c.failAt {
		c.seen++
		if c.seen == c.failOn {
			return broken
		}
	}
	separator := string(filepath.Separator)
	switch {
	case strings.Contains(path, separator+"journal"+separator):
		if journal, err := parseJournal(path, raw); err == nil {
			c.journals = append(c.journals, journal)
		}
	case strings.HasSuffix(path, ".json") && strings.Contains(path, separator+"inbox"+separator):
		c.writes[strings.TrimSuffix(filepath.Base(path), ".json")]++
	}
	return nil
}

// recorded says a journal state written since the n-th journal write holds
// id in the list field picks.
func (c *letterWrites) recorded(id string, since int, field func(TurnJournal) []string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, journal := range c.journals[min(since, len(c.journals)):] {
		if slices.Contains(field(journal), id) {
			return true
		}
	}
	return false
}

func (c *letterWrites) mooted(id string, since int) bool {
	return c.recorded(id, since, func(j TurnJournal) []string { return j.Moot })
}

func (c *letterWrites) journalWrites() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.journals)
}

func (c *letterWrites) of(id string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writes[id]
}

// Crashes: the sender is killed at each write of a clean publication's log,
// in turn, and the barriers after it run with r live, or after r has read
// what landed, ended and been swept by its next run. Across all attempts no
// letter is written twice.
func TestAKillAtAnyWriteLandsTheReportAtMostOnce(t *testing.T) {
	clean := newBareRaceLab(t)
	if err := clean.reconcile(t); err != nil {
		t.Fatal(err)
	}
	if writes := clean.letters.of(clean.report.ID); writes != 1 {
		t.Fatalf("the clean run wrote R %d times", writes)
	}
	writes := len(clean.letters.log)
	for _, swept := range []bool{false, true} {
		for at := 1; at <= writes; at++ {
			lab := newBareRaceLab(t)
			lab.letters.killAt = at
			if lab.reconcile(t) == nil {
				t.Errorf("killed at write %d (%s), the barrier answered nothing", at, clean.letters.log[at-1])
			}
			lab.letters.mu.Lock()
			lab.letters.killAt = 0
			lab.letters.mu.Unlock()
			if swept {
				lab.readAndSweep(t)
			}
			for range 2 {
				if err := lab.reconcile(t); err != nil {
					t.Errorf("killed at write %d (%s), swept %v: the retry answers %v", at, clean.letters.log[at-1], swept, err)
				}
			}
			if n := lab.letters.of(lab.report.ID); n > 1 {
				t.Errorf("killed at write %d (%s), swept %v: R was written %d times", at, clean.letters.log[at-1], swept, n)
			}
			if _, done := lab.journalOf(t); !done {
				t.Errorf("killed at write %d (%s), swept %v: the journal did not complete", at, clean.letters.log[at-1], swept)
			}
		}
	}
	t.Logf("killed at each of %d writes, with r live and after its sweep", writes)
}

// readAndSweep has r read R, if it landed, long enough ago, then ends r and
// lets the next run's sweep finish.
func (l raceLab) readAndSweep(t *testing.T) {
	t.Helper()
	unread := filepath.Join(state.InboxPath(l.dir, "lead"), l.report.ID+".json")
	if _, err := os.Stat(unread); err == nil {
		read := filepath.Join(state.DonePath(l.dir, "lead"), l.report.ID+".json")
		if err := state.EnsureSubdir(state.DonePath(l.dir, "lead")); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(unread, read); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-2 * keepFinished)
		if err := os.Chtimes(read, old, old); err != nil {
			t.Fatal(err)
		}
	}
	<-l.nextRunSweeps(t)
}
