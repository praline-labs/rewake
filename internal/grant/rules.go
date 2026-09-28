// Package grant decides which directories main may give a worker write access
// to with a task, and keeps the record of what rewake gave (docs/grants.md).
//
// Two tiers refuse a directory. The hard tier is what a directory may not
// equal, lie inside or contain: rewake's own state and binary, each harness's
// configuration, login keys, what runs as the person — PATH, git and systemd
// configuration — and the system directories. Nothing grants those. The broad
// tier needs confirming: a directory that holds far more than one task's
// work, a service's credentials, or where a live rewake session works, is
// granted only when main names it again with --grant-dir-broad.
package grant

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Rule is one protected directory and what it is.
type Rule struct {
	Path string
	// What completes "which is …" in a refusal.
	What string
	// Exact limits a hard rule to the directory itself and what contains it:
	// every path lies inside /.
	Exact bool
}

// Rules are the protected directories of one process's view of the machine,
// resolved as far as they exist.
type Rules struct {
	Hard  []Rule
	Broad []Rule
	// Home is the resolved home directory: each directory directly in it is
	// broad, and so is a directory directly in its .config holding a
	// credentials file.
	Home string
	// Sessions are where live rewake sessions work: a directory that is or
	// holds one is broad, since it holds that session's configuration.
	Sessions []string
	// Temp are the shared temporary directories, as named and as resolved: a
	// path given through one is refused wherever it leads.
	Temp []string
	// metadata judges the Git metadata --grant-git names: a shielded
	// directory is what that grant is for.
	metadata bool
}

// Refusal is a directory that cannot be granted, with the exit code the
// refusal carries: 2 for a call to change, 1 for a directory that is not
// there or not what it was.
type Refusal struct {
	Code    int
	Message string
}

func (r *Refusal) Error() string { return r.Message }

// Resolve turns a directory given on the command line into the path a
// harness is given: absolute from cwd, clean, links resolved, and a directory.
func Resolve(given, cwd string) (string, error) {
	path := given
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(path))
	if errors.Is(err, fs.ErrNotExist) {
		return "", &Refusal{Code: 1, Message: fmt.Sprintf("%s: no such directory; create it first, or check the path", given)}
	}
	if err != nil {
		return "", &Refusal{Code: 1, Message: fmt.Sprintf("%s: %v", given, err)}
	}
	resolved = longNames(resolved)
	info, err := os.Stat(resolved)
	if err != nil {
		return "", &Refusal{Code: 1, Message: fmt.Sprintf("%s: %v", given, err)}
	}
	if !info.IsDir() {
		return "", &Refusal{Code: 1, Message: fmt.Sprintf("%s is a file, not a directory; grant the directory that holds what the task writes", given)}
	}
	return resolved, nil
}

// Named refuses a directory given by a path through a shared temporary
// directory: a sandboxed worker can write there, and a link it put in place
// beforehand — /tmp/out leading to a directory of the owner's — would have
// main grant what the link leads to while naming a scratch directory. The
// hard tier refuses what lies there; this refuses what only passes through.
func (rules Rules) Named(given, cwd, resolved string) error {
	path := given
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	path = filepath.Clean(path)
	for _, temp := range rules.Temp {
		if within(path, temp) && !within(resolved, temp) {
			return &Refusal{Code: 2, Message: fmt.Sprintf("%s leads through %s, a shared temporary directory where a sandboxed worker can put a link, to %s; rewake grants no path through it. If %s is the directory the task needs, name it", given, temp, resolved, resolved)}
		}
	}
	return nil
}

// Check refuses a resolved directory the tiers protect. broad says main
// confirmed it with --grant-dir-broad; it is refused for a directory that is
// not broad, so the confirmation always names what it confirms.
func (rules Rules) Check(given, resolved string, broad bool) error {
	return rules.check(given, resolved, broad, true)
}

// check is Check; strict refuses a confirmation of a directory that is not
// broad. Only a call is strict: a directory broad when sent, for a session
// that has ended since, is no reason to refuse the task at delivery.
func (rules Rules) check(given, resolved string, broad, strict bool) error {
	shown := given
	if given != resolved {
		shown = fmt.Sprintf("%s (%s)", given, resolved)
	}
	if element, ok := shortElement(resolved); ok {
		return &Refusal{Code: 2, Message: fmt.Sprintf("%s goes through %s, a Windows short name rewake cannot match to its long one, and the checks compare long names; name the directory by its long name", shown, element)}
	}
	for _, rule := range rules.Hard {
		relation := ""
		switch {
		case same(resolved, rule.Path):
			relation = "is"
		case !rule.Exact && within(resolved, rule.Path):
			relation = "lies inside"
		case within(rule.Path, resolved):
			relation = "contains"
		}
		if relation != "" {
			target := rule.Path
			if relation == "is" {
				target = "the directory"
			}
			return &Refusal{Code: 2, Message: fmt.Sprintf("%s %s %s, which is %s; rewake never grants it. Grant a directory beside it, do that part of the work yourself, or ask the owner", shown, relation, target, rule.What)}
		}
	}
	what, isBroad := rules.broad(resolved)
	switch {
	case isBroad && !broad:
		return &Refusal{Code: 2, Message: fmt.Sprintf("%s is %s: a grant there reaches far more than one task needs. Grant the directory the task works in, or confirm this one with --grant-dir-broad %s", shown, what, given)}
	case !isBroad && broad && strict:
		return &Refusal{Code: 2, Message: fmt.Sprintf("--grant-dir-broad confirms a broad directory, and %s is none; pass it with --grant-dir", shown)}
	}
	return nil
}

// broad says whether a resolved directory is in the broad tier, and what it is.
func (rules Rules) broad(resolved string) (string, bool) {
	for _, rule := range rules.Broad {
		if same(resolved, rule.Path) {
			return rule.What, true
		}
	}
	if name, ok := shieldedElement(resolved); ok && !rules.metadata {
		// Inside a grant these stay with the person (Covers); granted as the
		// root, or below one, nothing inside would shield them, so the grant
		// has to name them.
		return fmt.Sprintf("inside %s, where a checkout keeps its metadata or a harness its configuration, and what reads it next runs what is written there", name), true
	}
	for _, session := range rules.Sessions {
		if within(session, resolved) {
			// Its .claude, .mcp.json and .rewake.toml tell a harness what to
			// run: a grant here reaches past the task, into that session.
			return "where a live rewake session works, or holds it, with that session's own configuration", true
		}
	}
	if rules.Home == "" {
		return "", false
	}
	if same(filepath.Dir(resolved), rules.Home) {
		return "a directory directly in the home directory", true
	}
	if same(filepath.Dir(resolved), resolve(filepath.Join(rules.Home, ".config"))) {
		if info, err := os.Lstat(filepath.Join(resolved, "credentials")); err == nil && !info.IsDir() {
			return "a service's configuration holding a credentials file", true
		}
	}
	return "", false
}

// CheckGitMetadata is Check for the Git metadata of a checkout --grant-git
// gives along with it: .git is what main named there, so being one is no
// reason for a confirmation; the rest of both tiers holds.
func (rules Rules) CheckGitMetadata(path string) error {
	rules.metadata = true
	return rules.check(path, path, false, false)
}

// Recheck is Check again at delivery, on a path that was resolved when it was
// sent: a link swapped in since then fails it, as does a directory gone.
func (rules Rules) Recheck(path string, broad bool) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return &Refusal{Code: 1, Message: fmt.Sprintf("%s is not an absolute clean path", path)}
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return &Refusal{Code: 1, Message: fmt.Sprintf("%s is gone: %v", path, err)}
	}
	if resolved = longNames(resolved); resolved != path {
		return &Refusal{Code: 1, Message: fmt.Sprintf("%s now leads to %s: a link changed since the grant was sent", path, resolved)}
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return &Refusal{Code: 1, Message: fmt.Sprintf("%s is no longer a directory", path)}
	}
	return rules.check(path, path, broad, false)
}

// Outermost drops every directory that lies inside another of the list, or
// repeats one, keeping the order of the rest.
func Outermost(paths []string) []string {
	var kept []string
	for i, path := range paths {
		covered := false
		for j, other := range paths {
			if i == j {
				continue
			}
			if within(path, other) && !same(path, other) || same(path, other) && j < i {
				covered = true
				break
			}
		}
		if !covered {
			kept = append(kept, path)
		}
	}
	return kept
}

// Within says whether path is dir or lies inside it, by path elements.
func Within(path, dir string) bool { return within(path, dir) }

func within(path, dir string) bool {
	rel, err := filepath.Rel(fold(dir), fold(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}

func same(a, b string) bool { return fold(a) == fold(b) }

// fold compares a path on a Windows drive without regard to case, as the
// drive itself does: /mnt/c/Users and /mnt/C/users are one directory.
func fold(path string) string {
	path = filepath.Clean(path)
	if windowsDrive(path) {
		return strings.ToLower(path)
	}
	return path
}

func isLetter(c byte) bool { return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' }

// ResolveExisting resolves as much of a path as exists, the rest joined on as
// given: a file a tool is about to create is judged where it will land.
func ResolveExisting(path string) string { return resolve(path) }

// shielded are the directories inside a grant a harness keeps its own
// configuration or a checkout its metadata in. A grant does not reach them,
// at any depth: a task that needs one gets it by name — a directory with one
// of them in its path is broad — or from the owner.
var shielded = []string{".git", ".claude", ".codex", ".agents"}

// Covers says whether a grant of root lets a session write path: path lies
// within root and in none of the directories shielded inside it.
func Covers(root, path string) bool {
	if !within(path, root) {
		return false
	}
	rel, _ := filepath.Rel(fold(root), fold(path))
	_, inside := shieldedElement(rel)
	return !inside
}

// shieldedElement names the first element of a path that is a shielded
// directory, in any case.
func shieldedElement(path string) (string, bool) {
	for _, element := range strings.Split(path, string(filepath.Separator)) {
		for _, name := range shielded {
			if strings.EqualFold(element, name) {
				return element, true
			}
		}
	}
	return "", false
}

// resolve resolves as much of a path as exists: a protected directory that is
// not there yet still protects where it would be, through the links above it.
func resolve(path string) string {
	path = filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return longNames(resolved)
	}
	parent := filepath.Dir(path)
	if parent == path {
		return path
	}
	return filepath.Join(resolve(parent), filepath.Base(path))
}
