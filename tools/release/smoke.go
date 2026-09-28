package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	runLimit      = 30 * time.Second
	shimTestLimit = 5 * time.Minute
)

// thisArch is this machine's npm cpu when a package serves it.
func thisArch() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	return map[string]string{"amd64": "x64", "arm64": "arm64"}[runtime.GOARCH]
}

// platformFor is the package serving an npm cpu.
func platformFor(arch string) (pkg, bool) {
	for _, p := range packages {
		if p.cpu != "" && p.cpu == arch {
			return p, true
		}
	}
	return pkg{}, false
}

// checkBinary runs this machine's binary: it has to name the version, and
// the build has to be HEAD with nothing changed, or the package is not the
// release its version says.
func (g *gate) checkBinary(ctx context.Context) {
	p, ok := platformFor(g.arch)
	if !ok {
		g.fail("no package serves %s/%s, so no binary can be run here: run the gate on linux x64 or arm64", runtime.GOOS, runtime.GOARCH)
		return
	}
	r := g.run(ctx, runLimit, g.root, filepath.Join(g.dist(p), "bin", "rewake"), "--version", "--json")
	if !r.ok() {
		g.fail("%s: rewake --version failed: %s", p.name, r.why())
		return
	}
	var build struct {
		Version  string `json:"version"`
		Known    bool   `json:"known"`
		Revision string `json:"revision"`
		Modified bool   `json:"modified"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &build); err != nil {
		g.fail("%s: rewake --version --json does not parse: %v", p.name, err)
		return
	}
	if build.Version != g.version {
		g.fail("%s: rewake --version says %s, want %s", p.name, build.Version, g.version)
	} else {
		g.pass("%s: rewake --version says %s", p.name, build.Version)
	}
	switch {
	case !build.Known:
		g.fail("%s: the binary carries no revision, so nobody can tell what it was built from", p.name)
	case g.head != "" && build.Revision != g.head:
		g.fail("%s: built from %s, not HEAD %s", p.name, short(build.Revision), short(g.head))
	case build.Modified:
		g.fail("%s: built from %s with local changes", p.name, short(build.Revision))
	default:
		g.pass("%s: built from HEAD %s with no local changes", p.name, short(build.Revision))
	}
}

// checkShim installs the entry the way npm lays it out, with and without this
// machine's platform package, and runs its command. How the shim finds a
// binary nested, hoisted or through the bin link is proven by the shim's own
// tests, which run here too; this proves the built packages fit together.
func (g *gate) checkShim(ctx context.Context) {
	p, ok := platformFor(g.arch)
	if !ok {
		return
	}
	entry := packages[len(packages)-1]
	root, err := os.MkdirTemp("", "rewake-release-")
	if err != nil {
		g.fail("smoke install: %v", err)
		return
	}
	defer func() { _ = os.RemoveAll(root) }()
	modules := filepath.Join(root, "node_modules", scope)
	platformDir := filepath.Join(modules, p.dir)
	if err := copyTree(g.dist(entry), filepath.Join(modules, entry.dir)); err != nil {
		g.fail("smoke install: %v", err)
		return
	}
	if err := copyTree(g.dist(p), platformDir); err != nil {
		g.fail("smoke install: %v", err)
		return
	}
	command := filepath.Join(modules, entry.dir, "bin", "rewake")

	r := g.run(ctx, runLimit, root, command, "--version")
	if want := "rewake " + g.version + " "; r.ok() && strings.HasPrefix(r.stdout, want) {
		g.pass("the shim runs %s's binary: %s", p.name, strings.TrimSpace(r.stdout))
	} else {
		g.fail("the shim with %s installed did not print %q: %s", p.name, want+"…", r.why())
	}

	if err := os.RemoveAll(platformDir); err != nil {
		g.fail("smoke install: %v", err)
		return
	}
	r = g.run(ctx, runLimit, root, command, "--version")
	if r.err == nil && r.code == 1 && strings.Contains(r.stderr, "no binary for") && strings.Contains(r.stderr, "npm install -g "+p.name) {
		g.pass("the shim without %s exits 1 naming the package to install", p.name)
	} else {
		g.fail("the shim without %s did not exit 1 with \"no binary for\" and the package to install: %s", p.name, r.why())
	}

	r = g.run(ctx, shimTestLimit, g.root, "go", "test", "-count=1", "./scripts/")
	if r.ok() {
		g.pass("the shim's own tests pass (go test ./scripts/)")
	} else {
		g.fail("the shim's own tests failed: %s", r.why())
	}
}

// copyTree copies a built package, keeping each file's mode: the command has
// to stay executable.
func copyTree(from, to string) error {
	return filepath.WalkDir(from, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", path)
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(from, to string, mode os.FileMode) error {
	source, err := os.Open(from)
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()
	target, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(target, source); err != nil {
		_ = target.Close()
		return err
	}
	return target.Close()
}
