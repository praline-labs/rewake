package codex

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/proc"
)

// lockDir holds one file per thread a Codex process is writing to.
const lockDir = "thread-writer-locks"

// CurrentThread returns the thread a running Codex session is writing to.
//
// Codex publishes this nowhere else: no flag assigns an id, and none is printed
// at startup. The process does hold a lock file named after the thread open from
// its first second, and after /new it holds the old one and the new one at once
// — so the answer is the most recently touched of them, not the only one.
func CurrentThread(harnessPID int, home string) (string, error) {
	if harnessPID == 0 {
		return "", fmt.Errorf("the codex process is not recorded yet")
	}
	// /proc reports resolved paths, so a CODEX_HOME that goes through a symlink
	// never matches the prefix unless it is resolved here too. Without this the
	// lock is found and rejected, and every message sits pending until it expires.
	resolved := home
	if absolute, err := filepath.Abs(resolved); err == nil {
		// /proc reports absolute, resolved paths. A relative CODEX_HOME never
		// matched, and every message sat pending until it expired.
		resolved = absolute
	}
	if link, err := filepath.EvalSymlinks(resolved); err == nil {
		resolved = link
	}
	prefix := filepath.Join(resolved, lockDir) + string(filepath.Separator)

	// The search goes outwards one generation at a time and stops at the first
	// one holding a thread. A child may be a Codex run of its own — a tool
	// calling `codex exec` — and its thread belongs to it, not to the session
	// somebody addressed. Taking the newest lock in the whole tree picked that
	// nested run whenever the command was a launcher without a lock of its own.
	generation := []int{harnessPID}
	seen := map[int]bool{harnessPID: true}
	for depth := 0; depth < 8 && len(generation) > 0; depth++ {
		if thread, err := threadOf(generation, prefix); err == nil {
			return thread, nil
		}

		var next []int
		for _, pid := range generation {
			children, err := proc.Children(pid)
			if err != nil {
				continue
			}
			for _, child := range children {
				if !seen[child] {
					seen[child] = true
					next = append(next, child)
				}
			}
		}
		generation = next
	}
	return "", fmt.Errorf("codex has not opened a thread yet")
}

// threadOf picks the most recently touched thread lock held by these processes.
func threadOf(processes []int, prefix string) (string, error) {

	type candidate struct {
		thread string
		at     time.Time
	}
	var candidates []candidate
	seen := map[string]bool{}

	for _, pid := range processes {
		files, err := proc.OpenFiles(pid)
		if err != nil {
			continue
		}
		for _, file := range files {
			if !strings.HasPrefix(file, prefix) || !strings.HasSuffix(file, ".lock") {
				continue
			}
			thread := strings.TrimSuffix(filepath.Base(file), ".lock")
			if thread == "" || seen[thread] {
				continue
			}
			seen[thread] = true

			info, err := os.Stat(file)
			if err != nil {
				continue
			}
			candidates = append(candidates, candidate{thread: thread, at: info.ModTime()})
		}
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("codex has not opened a thread yet")
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].at.After(candidates[j].at) })
	return candidates[0].thread, nil
}
