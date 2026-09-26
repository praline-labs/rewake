/*
Package worktree makes and keeps the checkouts rewake creates for a launch.

A harness that cannot make its own worktree under rewake — Codex's terminal
refuses its --worktree beside the --remote rewake always passes — gets one from
rewake instead: a detached checkout of the launch directory's HEAD, added with
the public `git worktree add`, and a record of whose it is. Nothing of a
harness's private layout is repeated: that layout is no contract, and a copy of
it would drift from the next version silently.

Where they live: one directory for every repository, outside any of them and
outside the state directory, which is under /tmp and would not outlive a
restart. Under it each repository has a directory named for it, and in that one
each checkout has its directory and its record side by side:

	<root>/<repository>-<hash>/<name>/        the checkout
	<root>/<repository>-<hash>/<name>.json    whose it is
*/
package worktree

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

// RootEnv moves the directory rewake keeps its worktrees in.
const RootEnv = "REWAKE_WORKTREES"

// Record says whose a checkout is: the repository it was made from, the commit
// it was made at, and the session it was made for.
type Record struct {
	Name string `json:"name"`
	// Repository is the directory under the root holding this repository's
	// checkouts: its name and a short hash of its Git directory.
	Repository string `json:"repository"`
	// CommonDir is the repository's shared Git directory, which knows every
	// worktree of it.
	CommonDir string `json:"commonDir"`
	// Source is the top of the checkout the launch was made from.
	Source string `json:"source"`
	Commit string `json:"commit"`
	Path   string `json:"path"`
	// Subdir is where the launch stood within Source, kept within the new
	// checkout as the harness's own worktree keeps it.
	Subdir    string    `json:"subdir,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	Session   *Owner    `json:"session,omitempty"`
}

// Owner is the session a checkout was made for.
type Owner struct {
	Name    string `json:"name"`
	Room    string `json:"room"`
	Epoch   string `json:"epoch"`
	Harness string `json:"harness"`
	// Dir is the room's state directory the session was registered in, where
	// a command asks whether it still runs: the state directory can be moved
	// per launch, and asking the current one could miss it.
	Dir string `json:"dir"`
}

// Ref is how a command names the checkout: its repository and its name.
func (r Record) Ref() string { return r.Repository + "/" + r.Name }

// Workdir is where a launch in this checkout starts: the launch directory's
// place within it, or its top when that directory is not in the commit — an
// untracked one, say.
func (r Record) Workdir() string {
	if r.Subdir != "" {
		within := filepath.Join(r.Path, r.Subdir)
		if info, err := os.Stat(within); err == nil && info.IsDir() {
			return within
		}
	}
	return r.Path
}

// Root is where rewake keeps its worktrees: RootEnv when set, else the user's
// data directory, which a restart does not clear.
func Root() (string, error) {
	if root := os.Getenv(RootEnv); root != "" {
		if !filepath.IsAbs(root) {
			return "", fmt.Errorf("%s must be an absolute path, got %q", RootEnv, root)
		}
		return filepath.Clean(root), nil
	}
	if data := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(data) {
		return filepath.Join(data, "rewake", "worktrees"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot find the home directory for the worktrees: %w; set %s", err, RootEnv)
	}
	return filepath.Join(home, ".local", "share", "rewake", "worktrees"), nil
}

// nameShape keeps a name one path element that cannot collide with a record's
// file name: no dot, so no name ends in .json.
var nameShape = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,39}$`)

// ValidName says whether a name may name a checkout.
func ValidName(name string) bool { return nameShape.MatchString(name) }

// NameRule says what ValidName accepts, for a refusal.
const NameRule = "a letter or digit, then up to 39 letters, digits, - or _"

// UnusableError is a checkout that cannot be made from what the call gave: a
// name out of shape, a directory outside any repository, a repository with no
// commit yet. The call has to change, not the repository.
type UnusableError struct{ Reason string }

func (e *UnusableError) Error() string { return e.Reason }

// ExistsError is a name already taken in a repository: by a checkout of
// rewake's, or by a directory in its place.
type ExistsError struct{ Record Record }

func (e *ExistsError) Error() string {
	return fmt.Sprintf("the worktree %s exists at %s", e.Record.Ref(), e.Record.Path)
}

// generatedTries bounds the search for a free generated name; a collision of
// six hex digits in one repository is rare, several in a row mean something
// else is wrong.
const generatedTries = 8

// Create adds a detached checkout of the HEAD of the repository holding from,
// named name or a generated name when name is empty, and records it.
func Create(root, from, name string) (Record, error) {
	if name != "" && !ValidName(name) {
		return Record{}, &UnusableError{Reason: fmt.Sprintf("the worktree name %q is not usable: %s", name, NameRule)}
	}
	source, err := inspect(from)
	if err != nil {
		return Record{}, err
	}
	if inside(resolved(root), resolved(source.Source)) {
		// Git would list the checkout among the repository's own files, and
		// an agent searching the repository would find a second copy of it.
		return Record{}, &UnusableError{Reason: fmt.Sprintf("the worktree directory %s is inside the repository at %s; set %s to a directory outside it", root, source.Source, RootEnv)}
	}
	directory := filepath.Join(root, source.Repository)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return Record{}, fmt.Errorf("cannot create %s: %w", directory, err)
	}
	record, err := add(source, directory, name)
	if err != nil {
		// Only when empty: a repository's directory holding other
		// checkouts stays.
		_ = os.Remove(directory)
	}
	return record, err
}

// add claims a name in directory and checks the source's commit out under it.
func add(source Record, directory, name string) (Record, error) {
	for try := 0; ; try++ {
		record := source
		record.Name = name
		if name == "" {
			record.Name = generatedName()
		}
		record.Path = filepath.Join(directory, record.Name)
		record.CreatedAt = time.Now().UTC()
		err := claim(record)
		var exists *ExistsError
		if name == "" && errors.As(err, &exists) && try < generatedTries {
			continue
		}
		if err != nil {
			return Record{}, err
		}
		if err := git(source.Source, "worktree", "add", "--detach", record.Path, record.Commit); err != nil {
			_ = os.Remove(recordPath(record))
			return Record{}, fmt.Errorf("git worktree add failed: %w", err)
		}
		return record, nil
	}
}

// claim publishes a record under its name, refusing a name that is taken: by a
// record, or by a directory standing where the checkout would go.
func claim(record Record) error {
	if _, err := os.Lstat(record.Path); err == nil {
		return &ExistsError{Record: record}
	}
	data, err := encode(record)
	if err != nil {
		return err
	}
	if err := state.PublishExclusive(recordPath(record), data); err != nil {
		if errors.Is(err, state.ErrNameTaken) {
			if held, readErr := read(recordPath(record)); readErr == nil {
				record = held
			}
			return &ExistsError{Record: record}
		}
		return err
	}
	return nil
}

// Claim writes the session a checkout was made for into its record.
func Claim(record Record, owner Owner) (Record, error) {
	record.Session = &owner
	data, err := encode(record)
	if err != nil {
		return record, err
	}
	return record, state.WriteAtomic(recordPath(record), data)
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
		if err != nil || recordPath(record) != path || filepath.Base(record.Path) != record.Name {
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

// Find returns the checkouts a command names: <repository>/<name> names one, a
// bare name every repository's checkout of that name.
func Find(root, ref string) ([]Record, error) {
	records, err := List(root)
	if err != nil {
		return nil, err
	}
	var found []Record
	for _, record := range records {
		if record.Ref() == ref || !strings.Contains(ref, "/") && record.Name == ref {
			found = append(found, record)
		}
	}
	return found, nil
}

func recordPath(record Record) string {
	return filepath.Join(filepath.Dir(record.Path), record.Name+".json")
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
	return record, nil
}

func encode(record Record) ([]byte, error) {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func generatedName() string {
	var raw [3]byte
	_, _ = rand.Read(raw[:])
	return hex.EncodeToString(raw[:])
}

// repositoryDir names a repository's directory under the root: the name a
// person knows it by, and a hash of its Git directory, so two repositories of
// one name do not share their checkouts' names.
func repositoryDir(commonDir string) string {
	name := filepath.Base(commonDir)
	if name == ".git" {
		name = filepath.Base(filepath.Dir(commonDir))
	}
	name = strings.TrimSuffix(name, ".git")
	clean := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '_'
	}, name)
	if clean == "" || strings.Trim(clean, ".") == "" {
		clean = "repository"
	}
	sum := sha256.Sum256([]byte(commonDir))
	return clean + "-" + hex.EncodeToString(sum[:3])
}

// resolved is a path with the symbolic links of its longest existing part
// resolved: a root not made yet is compared by where it would be.
func resolved(path string) string {
	rest := ""
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		if actual, err := filepath.EvalSymlinks(current); err == nil {
			return filepath.Join(actual, rest)
		}
		if parent := filepath.Dir(current); parent == current {
			return filepath.Clean(path)
		}
		rest = filepath.Join(filepath.Base(current), rest)
	}
}

// inside says whether path is dir or below it.
func inside(path, dir string) bool {
	relative, err := filepath.Rel(dir, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// Within says whether path lies in dir or is dir, symbolic links resolved.
func Within(path, dir string) bool {
	return path != "" && inside(resolved(path), resolved(dir))
}
