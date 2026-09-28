package main

import (
	"context"
	"debug/elf"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// scope is the npm organization the package is published under; the
// unscoped name belongs to somebody else.
const scope = "@praline-labs"

// packageName is the one name every build of a release is published under
// (owner decision, September 28, 2026): the platform builds are versions of it,
// not packages of their own.
const packageName = scope + "/rewake"

// pkg is one upload of a release, as scripts/pack.sh builds it.
type pkg struct {
	// dir is its directory under dist/npm.
	dir string
	// platform is a platform build's npm os and cpu, as its version suffix,
	// its dist-tag and its alias spell them; "" for the entry.
	platform string
	// cpu is the npm cpu of a platform build, "" for the entry.
	cpu string
	// machine is the architecture its binary must be built for.
	machine elf.Machine
	// files is everything the upload may carry, and must: an allowlist
	// checked both ways, since a missing file breaks the install and an
	// extra one stays in every mirror for as long as the version exists.
	files []string
}

// packages in the order they are published: the platform builds first, the
// entry, which names them, last.
var packages = []pkg{
	{dir: "rewake-linux-x64", platform: "linux-x64", cpu: "x64", machine: elf.EM_X86_64, files: []string{"LICENSE", "bin/rewake", "package.json"}},
	{dir: "rewake-linux-arm64", platform: "linux-arm64", cpu: "arm64", machine: elf.EM_AARCH64, files: []string{"LICENSE", "bin/rewake", "package.json"}},
	{dir: "rewake", files: []string{"LICENSE", "README.md", "bin/rewake", "package.json"}},
}

// version is the npm version this upload gets in a release: the release
// itself for the entry, the release with the platform as a prerelease suffix
// for a build, since npm takes one name and version only once.
func (p pkg) version(release string) string {
	if p.platform == "" {
		return release
	}
	return release + "-" + p.platform
}

// ref is what npm calls this upload in a release.
func (p pkg) ref(release string) string { return packageName + "@" + p.version(release) }

// tag is the dist-tag the upload goes under. A platform build has a tag of its
// own: published without one it would take latest, and the next install on
// any other machine would get a package with no command in it. The entry of
// a prerelease goes under next, so latest stays on the last release.
func (p pkg) tag(release string) string {
	switch {
	case p.platform != "":
		return p.platform
	case strings.Contains(release, "-"):
		return "next"
	default:
		return "latest"
	}
}

// alias is the name the entry installs a platform build under, and the one
// the shim looks for; scoped, so nothing from outside the organization can
// sit in its place.
func (p pkg) alias() string { return scope + "/rewake-" + p.platform }

const packLimit = time.Minute

// repository is where every package says its source lives.
const repository = "git+https://github.com/praline-labs/rewake.git"

// manifest is the part of package.json the gate checks.
type manifest struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	License    string `json:"license"`
	Repository struct {
		URL string `json:"url"`
	} `json:"repository"`
	OS                   []string          `json:"os"`
	CPU                  []string          `json:"cpu"`
	Bin                  map[string]string `json:"bin"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
}

// packed is the part of npm pack --dry-run --json the gate checks.
type packed struct {
	Size  int64 `json:"size"`
	Files []struct {
		Path string `json:"path"`
		Mode int    `json:"mode"`
	} `json:"files"`
}

// checkPackage checks one built upload: its manifest, what it would upload
// and, for a platform build, the architecture of its binary.
func (g *gate) checkPackage(ctx context.Context, p pkg) {
	dir, ref := g.dist(p), p.ref(g.version)
	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		g.fail("%s: %v", ref, err)
		return
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		g.fail("%s: package.json does not parse: %v", ref, err)
		return
	}
	if problems := manifestProblems(p, m, g.version); len(problems) > 0 {
		g.fail("%s: %s", ref, strings.Join(problems, "; "))
	} else if p.platform != "" {
		g.pass("%s: name, version, license, repository, os and cpu as expected, no bin", ref)
	} else {
		g.pass("%s: name, version, license, repository and bin as expected, the builds as its optional dependencies", ref)
	}

	r := g.run(ctx, packLimit, dir, "npm", "pack", "--dry-run", "--json", "--ignore-scripts")
	if !r.ok() {
		g.fail("%s: npm pack --dry-run failed: %s", ref, r.why())
		return
	}
	var answer []packed
	if err := json.Unmarshal([]byte(r.stdout), &answer); err != nil || len(answer) != 1 {
		g.fail("%s: npm pack --dry-run --json gave no single package: %v", ref, err)
		return
	}
	if problems := fileProblems(p, answer[0]); len(problems) > 0 {
		g.fail("%s: %s", ref, strings.Join(problems, "; "))
	} else {
		g.pass("%s: uploads exactly %s, %s packed", ref, strings.Join(p.files, ", "), size(answer[0].Size))
	}

	if p.cpu != "" {
		if err := checkMachine(filepath.Join(dir, "bin", "rewake"), p.machine); err != nil {
			g.fail("%s: %v", ref, err)
		} else {
			g.pass("%s: bin/rewake is a 64-bit %s executable", ref, p.machine)
		}
	}
}

// manifestProblems compares a package.json with what the upload must say.
func manifestProblems(p pkg, m manifest, release string) []string {
	var problems []string
	want := func(what, got, expected string) {
		if got != expected {
			problems = append(problems, fmt.Sprintf("%s is %q, want %q", what, got, expected))
		}
	}
	want("name", m.Name, packageName)
	want("version", m.Version, p.version(release))
	want("license", m.License, "MIT")
	want("repository", m.Repository.URL, repository)
	if p.cpu != "" {
		want("os", strings.Join(m.OS, ","), "linux")
		want("cpu", strings.Join(m.CPU, ","), p.cpu)
		// A build is installed beside the entry under an alias: a bin here
		// would be linked over the entry's command, and a dependency would
		// be one more thing every install fetches.
		if len(m.Bin) > 0 {
			problems = append(problems, "a platform build declares a bin, which would shadow the entry's")
		}
		if len(m.OptionalDependencies) > 0 {
			problems = append(problems, "a platform build declares optionalDependencies")
		}
		return problems
	}
	// The entry installs everywhere and picks its binary at run time: an os
	// or cpu here would refuse the install on the very machines it serves.
	if len(m.OS) > 0 || len(m.CPU) > 0 {
		problems = append(problems, "the entry declares os or cpu")
	}
	if !maps.Equal(m.Bin, map[string]string{"rewake": "bin/rewake"}) {
		problems = append(problems, fmt.Sprintf("bin is %v, want rewake: bin/rewake", m.Bin))
	}
	// Exactly this release's builds, each pinned by its whole version: a
	// range would let npm pick another release's binary.
	optional := map[string]string{}
	for _, build := range packages {
		if build.platform != "" {
			optional[build.alias()] = "npm:" + build.ref(release)
		}
	}
	if !maps.Equal(m.OptionalDependencies, optional) {
		problems = append(problems, fmt.Sprintf("optionalDependencies are %v, want %v", m.OptionalDependencies, optional))
	}
	return problems
}

// fileProblems compares what an upload would carry with its allowlist, both
// ways, and requires its command to be executable.
func fileProblems(p pkg, answer packed) []string {
	var problems []string
	shipped := map[string]int{}
	for _, file := range answer.Files {
		shipped[file.Path] = file.Mode
	}
	var missing, extra []string
	for _, file := range p.files {
		if _, ok := shipped[file]; !ok {
			missing = append(missing, file)
		}
	}
	for _, path := range slices.Sorted(maps.Keys(shipped)) {
		if !slices.Contains(p.files, path) {
			extra = append(extra, path)
		}
	}
	if len(missing) > 0 {
		problems = append(problems, "missing from the upload: "+strings.Join(missing, ", "))
	}
	if len(extra) > 0 {
		problems = append(problems, "would upload files nobody declared: "+strings.Join(extra, ", "))
	}
	if mode, ok := shipped["bin/rewake"]; ok && mode&0o111 == 0 {
		problems = append(problems, fmt.Sprintf("bin/rewake is not executable (mode %o)", mode))
	}
	return problems
}

// checkMachine reads the binary's ELF header: a package whose binary was
// built for the other architecture installs fine and then cannot run.
func checkMachine(path string, want elf.Machine) error {
	file, err := elf.Open(path)
	if err != nil {
		return fmt.Errorf("bin/rewake is no ELF executable: %w", err)
	}
	defer func() { _ = file.Close() }()
	if file.Class != elf.ELFCLASS64 || file.Machine != want {
		return fmt.Errorf("bin/rewake is built for %s %s, want %s", file.Class, file.Machine, want)
	}
	return nil
}

// size is a byte count as a person reads it.
func size(bytes int64) string {
	if bytes < 1e6 {
		return fmt.Sprintf("%.1f kB", float64(bytes)/1e3)
	}
	return fmt.Sprintf("%.1f MB", float64(bytes)/1e6)
}
