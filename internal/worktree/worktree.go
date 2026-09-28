/*
Package worktree makes and keeps the checkouts rewake creates for a launch.

A harness that cannot make its own worktree under rewake — Codex's terminal
refuses its --worktree beside the --remote rewake always passes — gets one from
rewake instead: a checkout of the launch directory's HEAD on a new branch of the
checkout's name, added with the public `git worktree add`, and a record of whose
it is. Nothing of a harness's private layout is repeated: that layout is no
contract, and a copy of it would drift from the next version silently.

The branch is how the work comes back: land fast-forwards the source's branch
to it, finish does that once more and removes the checkout and the branch.

Where they live: one directory for every repository, outside any of them and
outside the state directory, which is under /tmp and would not outlive a
restart. Under it each repository has a directory named for it, and in that one
each checkout has its directory and its record side by side:

	<root>/<repository>-<hash>/<name>/        the checkout
	<root>/<repository>-<hash>/<name>.json    whose it is

A name may hold slashes, as a branch's does; in these two it is written with +
for each, so every checkout is one directory beside the others.
*/
package worktree

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/state"
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
	// Branch is the branch made for the checkout, of its name. Empty in a
	// record of a build that checked out detached.
	Branch string `json:"branch,omitempty"`
	Path   string `json:"path"`
	// Subdir is where the launch stood within Source, kept within the new
	// checkout as the harness's own worktree keeps it.
	Subdir string `json:"subdir,omitempty"`
	// Included lists the files copied in from the source by
	// .worktreeinclude, relative to the top: ignored files, which rm would
	// otherwise take for work of the checkout's own.
	Included []string `json:"included,omitempty"`
	// Skipped says what .worktreeinclude named and could not be copied, for
	// the launch to tell; it is not kept.
	Skipped   []string  `json:"-"`
	CreatedAt time.Time `json:"createdAt"`
	// Launcher is the rewake process that made the checkout: until the
	// session is claimed, that it still runs is all that says the checkout is
	// about to be used.
	Launcher *Launch `json:"launcher,omitempty"`
	Session  *Owner  `json:"session,omitempty"`
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

// UnusableError is a checkout that cannot be made from what the call gave: a
// name out of shape, a directory outside any repository, a repository with no
// commit yet. The call has to change, not the repository.
type UnusableError struct{ Reason string }

func (e *UnusableError) Error() string { return e.Reason }

// StateError is a call that is right but meets a state that stops it: a
// repository or branch gone, a target another checkout holds, an operation
// under way. The state has to change, not the call.
type StateError struct{ Reason string }

func (e *StateError) Error() string { return e.Reason }

// ExistsError is a name already taken in a repository: by a checkout of
// rewake's, or by a directory in its place.
type ExistsError struct{ Record Record }

func (e *ExistsError) Error() string {
	return fmt.Sprintf("the worktree %s exists at %s", e.Record.Ref(), e.Record.Path)
}

// BranchTakenError is a name whose branch the repository already has: a
// checkout of it would not start where the launch stands, and landing it
// would take somebody else's commits. Below or Above is set when the name is
// free but git still cannot make it: a branch under it, <name>/..., or a branch
// its name is under, as feat is for feat/login. Git keeps a branch's name
// either as a branch or as a directory of branches, never both.
type BranchTakenError struct {
	Branch string
	Source string
	Below  string
	Above  string
}

func (e *BranchTakenError) Error() string {
	if e.Below != "" {
		return fmt.Sprintf("the repository at %s has a branch %s below the name %s, so git can make no branch %s", e.Source, e.Below, e.Branch, e.Branch)
	}
	if e.Above != "" {
		return fmt.Sprintf("the repository at %s has a branch %s, so git can make no branch %s below it", e.Source, e.Above, e.Branch)
	}
	return fmt.Sprintf("the repository at %s already has a branch %s", e.Source, e.Branch)
}

// generatedTries bounds the search for a free generated name; a collision of
// six hex digits in one repository is rare, several in a row mean something
// else is wrong.
const generatedTries = 8

// Create adds a checkout of the HEAD of the repository holding from on a new
// branch, both named name or a generated name when name is empty, copies in
// what .worktreeinclude asks for, and records it, holding the repository's
// lock throughout (serial.go).
func Create(root, from, name string) (Record, error) {
	if name != "" && !ValidName(name) {
		return Record{}, &UnusableError{Reason: fmt.Sprintf("the worktree name %q is not usable: %s", name, NameRule)}
	}
	source, err := inspect(from)
	if err != nil {
		return Record{}, err
	}
	// Git would list the checkout among the repository's own files, and an
	// agent searching the repository would find a second copy of it. The main
	// checkout is asked too: a launch from a linked one stands in another top.
	for _, top := range append([]string{source.Source}, mainTops(source.CommonDir)...) {
		if top != "" && inside(resolved(root), resolved(top)) {
			return Record{}, &UnusableError{Reason: fmt.Sprintf("the worktree directory %s is inside the repository at %s; set %s to a directory outside it", root, top, RootEnv)}
		}
	}
	directory := filepath.Join(root, source.Repository)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return Record{}, fmt.Errorf("cannot create %s: %w", root, err)
	}
	var record Record
	err = withRepositoryLock(directory, func() error {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("cannot create %s: %w", directory, err)
		}
		made, err := add(source, directory, name)
		if err != nil {
			// Only when empty: a repository's directory holding other
			// checkouts stays.
			_ = os.Remove(directory)
		}
		record = made
		return err
	})
	if err != nil {
		return Record{}, err
	}
	return record, nil
}

// add claims a name in directory and checks the source's commit out under it
// on a new branch of that name.
func add(source Record, directory, name string) (Record, error) {
	for try := 0; ; try++ {
		record := source
		record.Name = name
		if name == "" {
			record.Name = generatedName()
		}
		record.Branch = record.Name
		record.Path = filepath.Join(directory, flatName(record.Name))
		record.CreatedAt = time.Now().UTC()
		record.Launcher = launcher()
		err := claim(record)
		var exists *ExistsError
		var taken *BranchTakenError
		if name == "" && (errors.As(err, &exists) || errors.As(err, &taken)) && try < generatedTries {
			continue
		}
		if err != nil {
			return Record{}, err
		}
		if err := checkout(source, record); err != nil {
			return Record{}, err
		}
		return keepIncluded(include(record)), nil
	}
}

// unclaim takes back what a failed git worktree add left: git removes a
// checkout it could not finish, but the branch claim made stays. The branch is
// this call's own — claim made it and would have failed on one that existed —
// and it goes only while it is still at the commit and no checkout has it out;
// the directory goes only when empty.
func unclaim(record Record) {
	if tip, err := refTip(record.CommonDir, branchRef(record.Branch)); err == nil && tip == record.Commit {
		if at, err := checkedOutAt(record.CommonDir, branchRef(record.Branch)); err == nil && len(at) == 0 {
			_ = gitDir(record.CommonDir, "update-ref", "-d", branchRef(record.Branch), record.Commit)
		}
	}
	if info, err := os.Lstat(record.Path); err == nil && info.IsDir() {
		_ = os.Remove(record.Path)
	}
	_ = os.Remove(recordPath(record))
}

// claim publishes a record under its name and makes its branch, refusing a
// name that is taken: by a record, by a directory standing where the checkout
// would go, or by a branch. A checkout of the name is named first: its branch
// is taken too, and the checkout is what the person is looking for. The branch
// is made here, created only if absent in one step, rather than by git worktree
// add -b after a look: a branch somebody made between the look and the add
// would fail the add, and taking back the add would delete their branch.
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
	// An empty old value creates the ref only when there is none.
	err = gitDir(record.CommonDir, "update-ref", "-m", "branch: Created from "+record.Commit, branchRef(record.Branch), record.Commit, "")
	if err != nil {
		if tip, readErr := refTip(record.CommonDir, branchRef(record.Branch)); readErr == nil && tip != "" {
			err = &BranchTakenError{Branch: record.Branch, Source: record.Source}
		} else if below := branchBelow(record.CommonDir, record.Branch); below != "" {
			err = &BranchTakenError{Branch: record.Branch, Source: record.Source, Below: below}
		} else if above := branchAbove(record.CommonDir, record.Branch); above != "" {
			err = &BranchTakenError{Branch: record.Branch, Source: record.Source, Above: above}
		} else {
			err = fmt.Errorf("cannot make the branch %s: %w", record.Branch, err)
		}
		_ = os.Remove(recordPath(record))
		return err
	}
	return nil
}

// branchBelow names a branch under <branch>/, or "" when there is none or it
// cannot be told.
func branchBelow(commonDir, branch string) string {
	out, err := gitDirOutput(commonDir, "for-each-ref", "--count=1", "--format=%(refname:short)", branchRef(branch)+"/")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// branchAbove names a branch whose name leads the branch's, feat for
// feat/login, or "" when there is none or it cannot be told.
func branchAbove(commonDir, branch string) string {
	for at := strings.LastIndex(branch, "/"); at > 0; at = strings.LastIndex(branch[:at], "/") {
		if tip, err := refTip(commonDir, branchRef(branch[:at])); err == nil && tip != "" {
			return branch[:at]
		}
	}
	return ""
}

// generatedName is wt- and six hex digits: six digits alone would name a
// branch git takes for an abbreviated commit as well.
func generatedName() string {
	var raw [3]byte
	_, _ = rand.Read(raw[:])
	return "wt-" + hex.EncodeToString(raw[:])
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
// resolved: a root not made yet is compared by where it would be. A link to
// what does not exist yet is followed too, since that is where the root would
// be made.
func resolved(path string) string {
	return resolvedWithin(path, 40)
}

// resolvedWithin is resolved following at most hops dangling links, the most
// the kernel follows in one lookup, so a loop of them ends.
func resolvedWithin(path string, hops int) string {
	rest := ""
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		if actual, err := filepath.EvalSymlinks(current); err == nil {
			return filepath.Join(actual, rest)
		}
		if target, err := os.Readlink(current); err == nil && hops > 0 {
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(current), target)
			}
			return resolvedWithin(filepath.Join(target, rest), hops-1)
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
