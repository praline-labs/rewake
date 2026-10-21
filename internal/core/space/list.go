package space

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// What a view reads of the room's spaces: every record, the ones that do
// not read named apart, and the kept checkouts with what clears each.

// Problem is a file of the spaces directory that does not read as a record.
type Problem struct {
	Path string `json:"path"`
	Why  string `json:"why"`
}

// List reads every record of the room, in the order the spaces opened. It
// writes nothing and takes no lock, so a view may call it; a room with no
// spaces directory has no spaces. A record that does not read is a Problem
// beside the others, never a reason to show none.
func List(dir string) ([]Record, []Problem, error) {
	spaces := state.SpacesPath(dir)
	entries, err := state.ReadDirBelow(dir, filepath.Base(spaces))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var records []Record
	var problems []Problem
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue // the lock, and a write's temporary file
		}
		id, ok := strings.CutSuffix(name, ".json")
		if !ok || !ValidID(id) {
			problems = append(problems, Problem{Path: filepath.Join(spaces, name), Why: "not a space record"})
			continue
		}
		record, err := Load(dir, id)
		if err != nil {
			problems = append(problems, Problem{Path: state.SpacePath(dir, id), Why: err.Error()})
			continue
		}
		records = append(records, record)
	}
	slices.SortStableFunc(records, func(a, b Record) int {
		if c := a.OpenedAt.Compare(b.OpenedAt); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return records, problems, nil
}

// CheckoutPresence looks at the record's checkout now. Only a path that is
// missing for real is gone: any other failure to look is unknown.
func CheckoutPresence(r Record) Presence {
	if r.Checkout.Path == "" {
		return Gone
	}
	_, err := os.Lstat(r.Checkout.Path)
	switch {
	case err == nil:
		return There
	case errors.Is(err, os.ErrNotExist):
		return Gone
	}
	return Unknown
}

// Reminder is a kept checkout as main and the listing are told of it: where
// it is, what was found in it, why the close did not remove it, and the Git
// commands that clear it once what is wanted of it is merged. Each command
// refuses rather than lose work — a checkout with changes is not removed, an
// unmerged branch not deleted — so running them is safe; rewake runs none of
// them (SP7).
type Reminder struct {
	Space    string   `json:"space"`
	ID       string   `json:"id"`
	Path     string   `json:"path"`
	Branch   string   `json:"branch,omitempty"`
	Found    Seen     `json:"found"`
	Presence Presence `json:"presence"`
	// Stays is why the close kept it, as the space host recorded it.
	Stays string `json:"stays,omitempty"`
	// Clear are the commands, run from the checkout's repository.
	Clear []string `json:"clear"`
}

// Reminders are the kept checkouts among records that are not seen gone,
// in the records' order. A kept record whose checkout is gone is left out
// even before a change records it cleared: a view cannot write that, and
// nothing remains to remind.
func Reminders(records []Record) []Reminder {
	var out []Reminder
	for _, r := range records {
		if r.State != Kept {
			continue
		}
		presence := CheckoutPresence(r)
		if presence == Gone {
			continue
		}
		reminder := Reminder{Space: r.Name, ID: r.ID, Path: r.Checkout.Path, Branch: r.Checkout.Branch, Found: r.End.Checkout, Presence: presence}
		_, reminder.Stays = r.End.Reasons()
		if r.End.Checkout.Locked {
			reminder.Clear = append(reminder.Clear, "git worktree unlock "+shellWord(r.Checkout.Path))
		}
		reminder.Clear = append(reminder.Clear, "git worktree remove "+shellWord(r.Checkout.Path))
		if r.Checkout.Branch != "" {
			reminder.Clear = append(reminder.Clear, "git branch -d "+shellWord(r.Checkout.Branch))
		}
		out = append(out, reminder)
	}
	return out
}

var plainWord = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// shellWord quotes a word for a POSIX shell when it needs it.
func shellWord(word string) string {
	if plainWord.MatchString(word) {
		return word
	}
	return "'" + strings.ReplaceAll(word, "'", `'\''`) + "'"
}

// observeRun and namespace are the liveness checks, replaceable in tests.
var (
	observeRun = registry.ObserveRun
	namespace  = proc.Namespace
)

// MainRuns says whether the main run that opened a space still runs, has
// ended, or cannot be told from here: from another pid namespace, or one the
// record does not name, a pid says nothing.
func MainRuns(r Record) proc.IdentityEvidence {
	return runsHere(r, r.Main.Run)
}

// HostRuns says the same of the space host that closes the space, which runs
// in main's pid namespace: it is detached from main's tree, not moved out of
// its namespace. A host not yet recorded cannot be told.
func HostRuns(r Record) proc.IdentityEvidence {
	return runsHere(r, r.Host)
}

func runsHere(r Record, epoch string) proc.IdentityEvidence {
	here := namespace()
	if here == "" || r.Main.PIDNamespace != here || epoch == "" {
		return proc.IdentityUnknown
	}
	return observeRun(epoch)
}
