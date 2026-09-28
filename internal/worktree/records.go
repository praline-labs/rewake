package worktree

import (
	"encoding/json"
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// Claim writes the session a checkout was made for into its record.
func Claim(record Record, owner Owner) (Record, error) {
	record.Session = &owner
	data, err := encode(record)
	if err != nil {
		return record, err
	}
	return record, state.WriteAtomic(recordPath(record), data)
}

// keepIncluded writes what .worktreeinclude copied into the record at once,
// under the lock that made the checkout: until then an rm would take the
// copies for ignored files of the checkout's own, and a launch killed before
// its session is claimed would leave them so. Best effort: a record that
// keeps none only makes rm refuse more.
func keepIncluded(record Record) Record {
	if len(record.Included) > 0 {
		if data, err := encode(record); err == nil {
			_ = state.WriteAtomic(recordPath(record), data)
		}
	}
	return record
}

// Launch is the rewake process that made a checkout, with the pid namespace
// its pid means something in.
type Launch struct {
	// Epoch is pid.start; a start time that could not be read is 0, which
	// matches any.
	Epoch        string `json:"epoch"`
	PIDNamespace string `json:"pidNamespace,omitempty"`
}

// launcher is this process as a record names its launcher.
func launcher() *Launch {
	start, _ := proc.StartTime(os.Getpid())
	return &Launch{Epoch: strconv.Itoa(os.Getpid()) + "." + strconv.FormatUint(start, 10), PIDNamespace: proc.Namespace()}
}

// Judgeable says whether this process can tell if the launcher runs: only
// from the same pid namespace, as for a session (registry.Session.Judgeable).
// From another one — a sandbox — every pid but its own looks gone, and so
// does every pid when /proc cannot be read.
func (l Launch) Judgeable() bool {
	here := proc.Namespace()
	return here != "" && l.PIDNamespace == here
}

// Launching says whether the checkout is still being made or its session
// started, by a rewake process other than this one: the record names no
// session yet, and the process that made it still runs — or this process
// cannot tell that it does not, since taking such a launch for ended would
// remove the checkout it is about to start in.
func (r Record) Launching() bool {
	if r.Session != nil || r.Launcher == nil {
		return false
	}
	if !r.Launcher.Judgeable() {
		return true
	}
	pid, start, ok := registry.ParseEpoch(r.Launcher.Epoch)
	return ok && pid != os.Getpid() && proc.Alive(pid, start)
}

// mainTops are where git places a repository's main checkout, from its own
// data rather than from where the Git directory lies: core.worktree when the
// repository sets it, relative to the Git directory, and the first entry of
// git worktree list unless that is bare. With --separate-git-dir and no
// core.worktree, git 2.43 names the Git directory itself there (checked
// September 28, 2026): the main checkout is then written nowhere git keeps.
func mainTops(commonDir string) []string {
	var tops []string
	if configured, err := gitDirOutput(commonDir, "config", "--get", "core.worktree"); err == nil && configured != "" {
		if !filepath.IsAbs(configured) {
			configured = filepath.Join(commonDir, configured)
		}
		tops = append(tops, filepath.Clean(configured))
	}
	if list, err := gitDirOutput(commonDir, "worktree", "list", "--porcelain"); err == nil {
		first, _, _ := strings.Cut(list, "\n\n")
		lines := strings.Split(first, "\n")
		if top, ok := strings.CutPrefix(lines[0], "worktree "); ok && !slices.Contains(lines, "bare") {
			tops = append(tops, top)
		}
	}
	return tops
}

// List returns every checkout recorded under root, by repository and name. A
// record that cannot be read is left out: it names nothing a command could act
// on, and one bad file should not hide the rest.
func List(root string) ([]Record, error) {
	paths, err := filepath.Glob(filepath.Join(root, "*", "*.json"))
	if err != nil {
		return nil, err
	}
	var records []Record
	for _, path := range paths {
		record, err := read(path)
		if err != nil || recordPath(record) != path || filepath.Base(record.Path) != flatName(record.Name) {
			// A record whose checkout is not the directory of its name
			// beside it names a path rewake did not make, and rm would
			// remove it.
			continue
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Ref() < records[j].Ref() })
	return records, nil
}

// Find returns the checkouts a command names: <repository>/<name> names one,
// a bare name every repository's checkout of that name. A name may hold a
// slash itself, so a ref is read both ways, and the full form wins: the
// repository part is a directory rewake names with a hash, so a ref that
// matches one in full is meant that way, and one checkout's full form stays
// its own even when it spells another's name.
func Find(root, ref string) ([]Record, error) {
	records, err := List(root)
	if err != nil {
		return nil, err
	}
	var full, named []Record
	for _, record := range records {
		if record.Ref() == ref {
			full = append(full, record)
		} else if record.Name == ref {
			named = append(named, record)
		}
	}
	if len(full) > 0 {
		return full, nil
	}
	return named, nil
}

func recordPath(record Record) string {
	return filepath.Join(filepath.Dir(record.Path), flatName(record.Name)+".json")
}

func read(path string) (Record, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Record{}, err
	}
	var record Record
	if err := json.Unmarshal(raw, &record); err != nil {
		return Record{}, err
	}
	if !ValidName(record.Name) || !filepath.IsAbs(record.Path) || filepath.Clean(record.Path) != record.Path {
		return Record{}, fmt.Errorf("%s is not a worktree record", path)
	}
	// A branch other than the name would have land and finish move and
	// delete a branch rewake did not make; an included path leading out of
	// the checkout would have rm take a file there for a copy of its own.
	if record.Branch != "" && record.Branch != record.Name {
		return Record{}, fmt.Errorf("%s names the branch %q for the worktree %q", path, record.Branch, record.Name)
	}
	for _, file := range record.Included {
		if !filepath.IsLocal(file) || pathpkg.Clean(file) != file {
			return Record{}, fmt.Errorf("%s lists %q, which is not a path within the checkout", path, file)
		}
	}
	return record, nil
}

func encode(record Record) ([]byte, error) {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
