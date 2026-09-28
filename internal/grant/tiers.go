package grant

import (
	"os"
	"path/filepath"
	"slices"

	"github.com/praline-labs/rewake/internal/registry"
)

// Env is what the rules are built from.
type Env struct {
	Home       string
	PATH       string
	StateRoot  string
	Executable string
	// Harness names each harness's own configuration.
	Harness []string
	// Mounts are the mount points, of which those under /mnt and /media are
	// broad.
	Mounts []string
	// Sessions are the working directories of live rewake sessions.
	Sessions []string
	// Temp are the shared temporary directories.
	Temp []string
	// Caches are where the owner's tools keep built code and fetched
	// packages they load unchecked: GOMODCACHE and GOCACHE when set.
	Caches []string
}

// TempRoots are the shared temporary directories: /tmp and $TMPDIR. A Codex
// sandbox lets its commands write both by default, and the worker could put a
// link in place of a directory there once it is granted; the harness resolves
// a root again when it applies it (docs/grants.md#what-a-grant-does-not-stop).
// A variable only so that a test, whose every directory lies in one, can set
// them aside.
var TempRoots = func() []string { return []string{"/tmp", os.TempDir()} }

// CurrentEnv reads the environment of this process. harnessDirs come from
// the harness catalog, which this package does not import.
func CurrentEnv(stateRoot string, harnessDirs []string) Env {
	env := Env{PATH: os.Getenv("PATH"), StateRoot: stateRoot, Harness: harnessDirs}
	env.Home, _ = os.UserHomeDir()
	if executable, err := os.Executable(); err == nil {
		env.Executable = executable
	}
	env.Mounts = mountPoints("/proc/self/mountinfo")
	if stateRoot != "" {
		env.Sessions = registry.WorkDirs(stateRoot)
	}
	env.Temp = TempRoots()
	for _, name := range []string{"GOMODCACHE", "GOCACHE"} {
		if dir := os.Getenv(name); dir != "" {
			env.Caches = append(env.Caches, dir)
		}
	}
	return env
}

// systemDirs are refused inside and out; / only as itself.
var systemDirs = []string{"/etc", "/usr", "/bin", "/sbin", "/boot", "/dev", "/proc", "/sys", "/run", "/var", "/opt", "/root", "/srv", "/snap"}

// Rules builds the two tiers.
func (env Env) Rules() Rules {
	var rules Rules
	hard := func(path, what string) {
		if path != "" && filepath.IsAbs(path) {
			rules.Hard = append(rules.Hard, Rule{Path: resolve(path), What: what})
		}
	}
	hard(env.StateRoot, "rewake's state directory")
	if env.Executable != "" {
		hard(filepath.Dir(resolve(env.Executable)), "the directory of the rewake binary")
	}
	for _, dir := range env.Harness {
		hard(dir, "a harness's own configuration")
	}
	if env.Home != "" {
		hard(filepath.Join(env.Home, ".config", "rewake"), "rewake's configuration")
		for _, name := range []string{".ssh", ".gnupg", ".aws", ".kube", ".docker", ".password-store", ".config/gh"} {
			hard(filepath.Join(env.Home, name), "where login keys are kept")
		}
		for _, name := range []string{".config/git", ".config/systemd", ".config/autostart", ".config/environment.d", ".config/fish", ".local/share/systemd", ".local/share/applications"} {
			hard(filepath.Join(env.Home, name), "configuration that runs or signs as the owner")
		}
		for _, name := range []string{"go/pkg/mod", ".cache/go-build"} {
			hard(filepath.Join(env.Home, name), "code the owner's tools load and run")
		}
		rules.Home = resolve(env.Home)
	}
	for _, dir := range env.Caches {
		hard(dir, "code the owner's tools load and run")
	}
	for _, dir := range filepath.SplitList(env.PATH) {
		hard(dir, "a directory on PATH")
		if toolchain := toolchainOf(dir, env.Home); toolchain != "" {
			hard(toolchain, "a toolchain whose programs are on PATH, with the libraries they load")
		}
	}
	for _, dir := range env.Temp {
		hard(dir, "a shared temporary directory, where a sandboxed worker can write and swap a granted directory for a link")
		if filepath.IsAbs(dir) {
			rules.Temp = append(rules.Temp, filepath.Clean(dir), resolve(dir))
		}
	}
	rules.Hard = append(rules.Hard, Rule{Path: "/", What: "the root of the filesystem", Exact: true})
	libs, _ := filepath.Glob("/lib*")
	for _, dir := range append(slices.Clone(systemDirs), append([]string{"/lib"}, libs...)...) {
		hard(dir, "a system directory")
	}

	for _, dir := range []string{"/home", "/mnt", "/media"} {
		rules.Broad = append(rules.Broad, Rule{Path: resolve(dir), What: "the directory of every home or drive"})
	}
	for _, mount := range env.Mounts {
		if within(mount, "/mnt") || within(mount, "/media") {
			rules.Broad = append(rules.Broad, Rule{Path: resolve(mount), What: "a mounted drive"})
		}
	}
	for _, dir := range env.Sessions {
		if filepath.IsAbs(dir) {
			rules.Sessions = append(rules.Sessions, resolve(dir))
		}
	}
	return rules
}

// toolchainOf is the directory a bin on PATH belongs to — a Go, Node or Rust
// installation, a GOPATH — whose libraries and packages its programs load, so
// writing them is running as the owner. The home directory and ~/.local are no
// toolchain, and neither is /: they hold far more than one.
func toolchainOf(dir, home string) string {
	if !filepath.IsAbs(dir) || filepath.Base(filepath.Clean(dir)) != "bin" {
		return ""
	}
	parent := filepath.Dir(filepath.Clean(dir))
	if parent == "/" || home != "" && (same(parent, home) || same(parent, filepath.Join(home, ".local"))) {
		return ""
	}
	return parent
}
