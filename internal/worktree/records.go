package worktree

import (
	"encoding/json"
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strconv"

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

// launcher is this process as a record names its launcher, or "" when its
// start time cannot be read.
func launcher() string {
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		return ""
	}
	return strconv.Itoa(os.Getpid()) + "." + strconv.FormatUint(start, 10)
}

// Launching says whether the checkout is still being made or its session
// started, by a rewake process other than this one: the record names no
// session yet, and the process that made it still runs.
func (r Record) Launching() bool {
	if r.Session != nil {
		return false
	}
	pid, start, ok := registry.ParseEpoch(r.Launcher)
	return ok && pid != os.Getpid() && proc.Alive(pid, start)
}

// mainCheckout is the checkout holding a repository's shared Git directory,
// or "" for one kept apart from any, a bare repository's.
func mainCheckout(commonDir string) string {
	if filepath.Base(commonDir) != ".git" {
		return ""
	}
	return filepath.Dir(commonDir)
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
