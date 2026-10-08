// Package cache downloads a harness version from the npm registry once and
// keeps it unpacked, keyed by harness and exact version, so that the next run
// against the same version touches no network at all.
//
// It exists to answer one question before the owner updates their own
// installation: what does the next version break? So it never reads or writes
// the owner's installation — the one thing it does with an installed binary is
// ask it for its version, when the selector says "installed" — and it never
// removes anything on its own: a cached version goes only when somebody asks.
//
// It is a package rather than code inside the tool because two callers need
// it: the tool a person or agent runs, and the workflow suite, which fetches
// the version named in its environment before its cases start. The tier that
// runs a real harness against a local responder will be a third.
package cache

import (
	"fmt"
	"regexp"
	"runtime"
	"sort"
)

// Harness describes where one harness is published and what in its package is
// the executable.
//
// The two harnesses publish differently, which is why this is a description
// rather than a naming rule. Codex publishes its platform build as a version
// of the same package with a platform suffix (0.155.1-linux-x64). Claude Code
// publishes a small wrapper whose postinstall copies a binary out of a
// separate platform package; here the platform package is fetched directly,
// because running that postinstall would need node and would run a script
// from the download.
type Harness struct {
	// Name is the harness as rewake names it, and the command that answers
	// --version for the "installed" selector.
	Name string
	// Package carries the dist-tags: "latest" is resolved against it.
	Package string
	// platform names the package and version holding the build for one
	// architecture.
	platform func(version, arch string) (pkg, pkgVersion string)
	// executable is the path of the binary inside the unpacked tarball.
	executable func(arch string) string
}

// Build is the package holding one version for this machine.
func (h Harness) Build(version string) (pkg, pkgVersion string, err error) {
	arch, err := npmArch()
	if err != nil {
		return "", "", err
	}
	pkg, pkgVersion = h.platform(version, arch)
	return pkg, pkgVersion, nil
}

// Executable is the binary's path relative to a version's directory.
func (h Harness) Executable() (string, error) {
	arch, err := npmArch()
	if err != nil {
		return "", err
	}
	return h.executable(arch), nil
}

var harnesses = map[string]Harness{
	"codex": {
		Name:    "codex",
		Package: "@openai/codex",
		platform: func(version, arch string) (string, string) {
			return "@openai/codex", version + "-linux-" + arch
		},
		executable: func(arch string) string {
			return "package/vendor/" + rustTriple[arch] + "/bin/codex"
		},
	},
	"claude": {
		Name:    "claude",
		Package: "@anthropic-ai/claude-code",
		// The musl build rather than the glibc one: it runs in the small
		// container image the suite uses, and the version number is the same
		// either way.
		platform: func(version, arch string) (string, string) {
			return "@anthropic-ai/claude-code-linux-" + arch + "-musl", version
		},
		executable: func(string) string { return "package/claude" },
	},
}

// rustTriple is the directory Codex puts its binary under for each
// architecture it publishes for Linux.
var rustTriple = map[string]string{
	"x64":   "x86_64-unknown-linux-musl",
	"arm64": "aarch64-unknown-linux-musl",
}

// Lookup finds a harness by name; the refusal lists the names it knows.
func Lookup(name string) (Harness, error) {
	h, ok := harnesses[name]
	if !ok {
		return Harness{}, fmt.Errorf("no harness %q; known: %v", name, Names())
	}
	return h, nil
}

// Names are the harnesses this package can fetch, sorted.
func Names() []string {
	names := make([]string, 0, len(harnesses))
	for name := range harnesses {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// npmArch is the architecture as the npm packages spell it. Only Linux is
// published in the shape this package reads, and only these two.
func npmArch() (string, error) {
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("harness builds are fetched for linux only, and this is %s", runtime.GOOS)
	}
	switch runtime.GOARCH {
	case "amd64":
		return "x64", nil
	case "arm64":
		return "arm64", nil
	default:
		return "", fmt.Errorf("no harness build is published for %s", runtime.GOARCH)
	}
}

// exactVersion is what may name a directory in the cache. It is checked
// before a version becomes a path, so a selector cannot climb out of the
// cache with a slash or two dots.
var exactVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$`)

// IsExact reports whether a selector is an exact version.
func IsExact(selector string) bool { return exactVersion.MatchString(selector) }
