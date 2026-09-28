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

// scope is the npm organization the packages are published under; the
// unscoped name belongs to somebody else.
const scope = "@praline-labs"

// pkg is one npm package of a release, as scripts/pack.sh builds it.
type pkg struct {
	// dir is the package's directory under dist/npm.
	dir  string
	name string
	// cpu is the npm cpu of a platform package, "" for the entry.
	cpu string
	// machine is the architecture its binary must be built for.
	machine elf.Machine
	// files is everything the package may upload, and must: an allowlist
	// checked both ways, since a missing file breaks the install and an
	// extra one stays in every mirror for as long as the version exists.
	files []string
}

// packages in the order they are published: the platforms first, the entry,
// which names them, last.
var packages = []pkg{
	{dir: "rewake-linux-x64", name: scope + "/rewake-linux-x64", cpu: "x64", machine: elf.EM_X86_64, files: []string{"LICENSE", "bin/rewake", "package.json"}},
	{dir: "rewake-linux-arm64", name: scope + "/rewake-linux-arm64", cpu: "arm64", machine: elf.EM_AARCH64, files: []string{"LICENSE", "bin/rewake", "package.json"}},
	{dir: "rewake", name: scope + "/rewake", files: []string{"LICENSE", "README.md", "bin/rewake", "package.json"}},
}

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

// checkPackage checks one built package: its manifest, what it would upload
// and, for a platform package, the architecture of its binary.
func (g *gate) checkPackage(ctx context.Context, p pkg) {
	dir := g.dist(p)
	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		g.fail("%s: %v", p.name, err)
		return
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		g.fail("%s: package.json does not parse: %v", p.name, err)
		return
	}
	if problems := manifestProblems(p, m, g.version); len(problems) > 0 {
		g.fail("%s: %s", p.name, strings.Join(problems, "; "))
	} else {
		g.pass("%s: name, version, license, repository, os and cpu as expected", p.name)
	}

	r := g.run(ctx, packLimit, dir, "npm", "pack", "--dry-run", "--json", "--ignore-scripts")
	if !r.ok() {
		g.fail("%s: npm pack --dry-run failed: %s", p.name, r.why())
		return
	}
	var answer []packed
	if err := json.Unmarshal([]byte(r.stdout), &answer); err != nil || len(answer) != 1 {
		g.fail("%s: npm pack --dry-run --json gave no single package: %v", p.name, err)
		return
	}
	if problems := fileProblems(p, answer[0]); len(problems) > 0 {
		g.fail("%s: %s", p.name, strings.Join(problems, "; "))
	} else {
		g.pass("%s: uploads exactly %s, %s packed", p.name, strings.Join(p.files, ", "), size(answer[0].Size))
	}

	if p.cpu != "" {
		if err := checkMachine(filepath.Join(dir, "bin", "rewake"), p.machine); err != nil {
			g.fail("%s: %v", p.name, err)
		} else {
			g.pass("%s: bin/rewake is a 64-bit %s executable", p.name, p.machine)
		}
	}
}

// manifestProblems compares a package.json with what the package must say.
func manifestProblems(p pkg, m manifest, version string) []string {
	var problems []string
	want := func(what, got, expected string) {
		if got != expected {
			problems = append(problems, fmt.Sprintf("%s is %q, want %q", what, got, expected))
		}
	}
	want("name", m.Name, p.name)
	want("version", m.Version, version)
	want("license", m.License, "MIT")
	want("repository", m.Repository.URL, repository)
	if p.cpu != "" {
		want("os", strings.Join(m.OS, ","), "linux")
		want("cpu", strings.Join(m.CPU, ","), p.cpu)
		if len(m.Bin) > 0 {
			problems = append(problems, "a platform package declares a bin, which would shadow the entry's")
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
	optional := map[string]string{}
	for _, other := range packages {
		if other.cpu != "" {
			optional[other.name] = version
		}
	}
	if !maps.Equal(m.OptionalDependencies, optional) {
		problems = append(problems, fmt.Sprintf("optionalDependencies are %v, want %v", m.OptionalDependencies, optional))
	}
	return problems
}

// fileProblems compares what a package would upload with its allowlist, both
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
