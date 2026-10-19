package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/adapter/pi"
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

// platformFor is the build serving an npm cpu.
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
		g.fail("no build serves %s/%s, so no binary can be run here: run the gate on linux x64 or arm64", runtime.GOOS, runtime.GOARCH)
		return
	}
	ref := p.ref(g.version)
	r := g.run(ctx, runLimit, g.root, filepath.Join(g.dist(p), "bin", "rewake"), "--version", "--json")
	if !r.ok() {
		g.fail("%s: rewake --version failed: %s", ref, r.why())
		return
	}
	var build struct {
		Version  string `json:"version"`
		Known    bool   `json:"known"`
		Revision string `json:"revision"`
		Modified bool   `json:"modified"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &build); err != nil {
		g.fail("%s: rewake --version --json does not parse: %v", ref, err)
		return
	}
	if build.Version != g.version {
		g.fail("%s: rewake --version says %s, want %s", ref, build.Version, g.version)
	} else {
		g.pass("%s: rewake --version says %s", ref, build.Version)
	}
	switch {
	case !build.Known:
		g.fail("%s: the binary carries no revision, so nobody can tell what it was built from", ref)
	case g.head != "" && build.Revision != g.head:
		g.fail("%s: built from %s, not HEAD %s", ref, short(build.Revision), short(g.head))
	case build.Modified:
		g.fail("%s: built from %s with local changes", ref, short(build.Revision))
	default:
		g.pass("%s: built from HEAD %s with no local changes", ref, short(build.Revision))
	}
}

// checkInstall has npm install the release from a registry in this process,
// as it will from the public one: the entry resolves its aliases, npm keeps
// the build whose os and cpu match and leaves the others, and the command
// runs. Then each other build as npm would choose it on its own machine, and
// the entry without optional dependencies, whose command has to say how to
// get its binary. How the shim finds a binary nested, hoisted or through the
// bin link is proven by the shim's own tests, which run here too.
func (g *gate) checkInstall(ctx context.Context) {
	p, ok := platformFor(g.arch)
	if !ok {
		return
	}
	scratch, err := os.MkdirTemp("", "rewake-release-")
	if err != nil {
		g.fail("install from a local registry: %v", err)
		return
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	registry, err := g.serveRelease(ctx, scratch)
	if err != nil {
		g.fail("install from a local registry: %v", err)
		return
	}
	defer registry.close()

	command, builds, ok := g.install(ctx, registry, scratch, "this-machine", "--global", "--include=optional")
	if ok && g.onlyBuild(builds, p, "this machine") {
		r := g.run(ctx, runLimit, scratch, command, "--version")
		if want := "rewake " + g.version + " "; r.ok() && strings.HasPrefix(r.stdout, want) {
			g.pass("the installed command runs %s: %s", p.ref(g.version), strings.TrimSpace(r.stdout))
		} else {
			g.fail("the installed command did not print %q: %s", want+"…", r.why())
		}
	}
	if ok {
		modules := filepath.Join(scratch, "this-machine", "lib", "node_modules")
		g.shippedPi(ctx, modules, scratch)
		g.installedShell(ctx, modules, p, scratch)
	}
	for _, other := range packages {
		if other.platform == "" || other.platform == p.platform {
			continue
		}
		if _, builds, ok := g.install(ctx, registry, scratch, other.platform, "--global", "--include=optional", "--os=linux", "--cpu="+other.cpu); ok {
			g.onlyBuild(builds, other, "--os=linux --cpu="+other.cpu)
		}
	}

	// Into a project, not --global: npm 11 installs a global package's
	// optional dependencies even under --omit=optional.
	command, builds, ok = g.install(ctx, registry, scratch, "no-optional", "--omit=optional")
	if ok {
		r := g.run(ctx, runLimit, scratch, command, "--version")
		reinstall := "npm install -g " + packageName + "\n"
		if len(builds) == 0 && r.err == nil && r.code == 1 && strings.Contains(r.stderr, "no binary for") && strings.Contains(r.stderr, reinstall) {
			g.pass("with --omit=optional no build is installed, and the command exits 1 naming the reinstall")
		} else {
			g.fail("with --omit=optional npm installed %v, and the command did not exit 1 with \"no binary for\" and %q: %s", builds, strings.TrimSpace(reinstall), r.why())
		}
	}

	r := g.run(ctx, shimTestLimit, g.root, "go", "test", "-count=1", "./scripts/")
	if r.ok() {
		g.pass("the shim's own tests pass (go test ./scripts/)")
	} else {
		g.fail("the shim's own tests failed: %s", r.why())
	}
}

// installLimit bounds one npm install from the local registry. The release's
// own packages are local; pi and its dependencies come from the public
// registry once and from the scratch cache after.
const installLimit = 3 * time.Minute

// install runs npm install of the release's entry into a prefix of its own,
// against the local registry only, and returns the installed command and the
// builds npm put beside it, by alias and version. The cache and both npm
// configuration files are fresh, so neither the owner's settings nor an
// earlier download answers in the registry's place.
func (g *gate) install(ctx context.Context, registry *localRegistry, scratch, name string, flags ...string) (string, map[string]string, bool) {
	prefix := filepath.Join(scratch, name)
	// npm refuses one file named as both.
	user, global := filepath.Join(scratch, "user-npmrc"), filepath.Join(scratch, "global-npmrc")
	for _, config := range []string{user, global} {
		if err := os.WriteFile(config, nil, 0o600); err != nil {
			g.fail("install %s: %v", name, err)
			return "", nil, false
		}
	}
	args := []string{
		"install", "--prefix", prefix, "--cache", filepath.Join(scratch, "cache"),
		"--userconfig", user, "--globalconfig", global,
		"--registry", registry.url, "--" + scope + ":registry=" + registry.url,
		"--noproxy", "127.0.0.1", "--no-audit", "--no-fund", "--ignore-scripts",
	}
	args = append(append(args, flags...), packageName+"@"+g.version)
	r := g.run(ctx, installLimit, scratch, "npm", args...)
	if !r.ok() {
		g.fail("npm install %s %s from a local registry failed: %s", strings.Join(flags, " "), packageName+"@"+g.version, r.why())
		return "", nil, false
	}
	modules, command := filepath.Join(prefix, "node_modules"), filepath.Join(prefix, "node_modules", ".bin", "rewake")
	if slices.Contains(flags, "--global") {
		modules, command = filepath.Join(prefix, "lib", "node_modules"), filepath.Join(prefix, "bin", "rewake")
	}
	builds, err := installedBuilds(modules)
	if err != nil {
		g.fail("install %s: %v", name, err)
		return "", nil, false
	}
	return command, builds, true
}

// onlyBuild requires npm to have installed exactly the one build, under its
// alias and at this release's version.
func (g *gate) onlyBuild(builds map[string]string, p pkg, machine string) bool {
	if len(builds) == 1 && builds[p.alias()] == p.version(g.version) {
		g.pass("npm installs %s for %s, as %s, and no other build", p.ref(g.version), machine, p.alias())
		return true
	}
	g.fail("for %s npm installed %v, want only %s at %s", machine, builds, p.alias(), p.version(g.version))
	return false
}

// installedBuilds finds every platform build under a node_modules tree,
// wherever npm placed it, by the alias directory it sits in.
func installedBuilds(modules string) (map[string]string, error) {
	builds := map[string]string{}
	err := filepath.WalkDir(modules, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || entry.Name() != "package.json" {
			return err
		}
		dir := filepath.Dir(path)
		alias := filepath.Base(filepath.Dir(dir)) + "/" + filepath.Base(dir)
		if !strings.HasPrefix(alias, scope+"/rewake-") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var m manifest
		if err := json.Unmarshal(raw, &m); err != nil {
			return fmt.Errorf("%s does not parse: %w", path, err)
		}
		builds[alias] = m.Version
		return nil
	})
	return builds, err
}

// shippedPi requires npm to have put the pinned pi where the installed binary
// looks for it, a node_modules above its own directory, and that copy to
// answer its version under node, as a launch reads it.
func (g *gate) shippedPi(ctx context.Context, modules, scratch string) {
	for _, dir := range []string{filepath.Join(modules, packageName, "node_modules"), modules} {
		manifest := filepath.Join(dir, pi.Package, "package.json")
		raw, err := os.ReadFile(manifest)
		if err != nil {
			continue
		}
		var found struct {
			Bin map[string]string `json:"bin"`
		}
		if err := json.Unmarshal(raw, &found); err != nil || found.Bin["pi"] == "" {
			g.fail("%s names no pi command: %v", manifest, err)
			return
		}
		// pi reads its settings before it answers, so it gets an agent
		// directory of the scratch's, never the person's own pi's.
		r := g.run(ctx, runLimit, scratch, "env", pi.AgentDirEnv+"="+filepath.Join(scratch, "pi-agent"), pi.SkipVersionCheckEnv+"=1",
			"node", filepath.Join(dir, pi.Package, found.Bin["pi"]), "--version")
		if r.ok() && strings.TrimSpace(r.stdout) == pi.PinnedVersion {
			g.pass("npm installs %s@%s beside the command, and it runs under node", pi.Package, pi.PinnedVersion)
		} else {
			g.fail("the installed %s is not %s under node: %s %s", pi.Package, pi.PinnedVersion, strings.TrimSpace(r.stdout), r.why())
		}
		return
	}
	g.fail("npm installed no %s beside the command, where a launch of pi looks for it", pi.Package)
}
