package main

import (
	"debug/elf"
	"os"
	"strings"
	"testing"
)

// Every upload is the one package at its own version; a build names its os
// and cpu and no command, and the entry pins exactly this release's builds
// through their aliases.
func TestManifestProblems(t *testing.T) {
	x64, entry := packages[0], packages[2]
	good := manifest{Name: packageName, Version: "1.0.0-linux-x64", License: "MIT", OS: []string{"linux"}, CPU: []string{"x64"}}
	good.Repository.URL = repository
	if problems := manifestProblems(x64, good, "1.0.0"); len(problems) > 0 {
		t.Errorf("a right platform manifest: %v", problems)
	}
	for name, change := range map[string]func(*manifest){
		"cpu":                  func(m *manifest) { m.CPU = []string{"arm64"} },
		"name":                 func(m *manifest) { m.Name = scope + "/rewake-linux-x64" },
		"version":              func(m *manifest) { m.Version = "1.0.0" },
		"bin":                  func(m *manifest) { m.Bin = map[string]string{"rewake": "bin/rewake"} },
		"optionalDependencies": func(m *manifest) { m.OptionalDependencies = map[string]string{"x": "1"} },
	} {
		wrong := good
		change(&wrong)
		if problems := manifestProblems(x64, wrong, "1.0.0"); len(problems) != 1 || !strings.Contains(problems[0], name) {
			t.Errorf("a wrong %s: %v", name, problems)
		}
	}

	entryGood := manifest{
		Name: packageName, Version: "1.0.0", License: "MIT", Bin: map[string]string{"rewake": "bin/rewake"},
		OptionalDependencies: map[string]string{
			scope + "/rewake-linux-x64":   "npm:" + packageName + "@1.0.0-linux-x64",
			scope + "/rewake-linux-arm64": "npm:" + packageName + "@1.0.0-linux-arm64",
		},
	}
	entryGood.Repository.URL = repository
	if problems := manifestProblems(entry, entryGood, "1.0.0"); len(problems) > 0 {
		t.Errorf("a right entry manifest: %v", problems)
	}
	for name, optional := range map[string]map[string]string{
		"the old separate packages": {scope + "/rewake-linux-x64": "1.0.0", scope + "/rewake-linux-arm64": "1.0.0"},
		"a range":                   {scope + "/rewake-linux-x64": "npm:" + packageName + "@^1.0.0-linux-x64", scope + "/rewake-linux-arm64": "npm:" + packageName + "@1.0.0-linux-arm64"},
		"another release's build":   {scope + "/rewake-linux-x64": "npm:" + packageName + "@0.9.0-linux-x64", scope + "/rewake-linux-arm64": "npm:" + packageName + "@1.0.0-linux-arm64"},
		"a build missing":           {scope + "/rewake-linux-x64": "npm:" + packageName + "@1.0.0-linux-x64"},
		"an unscoped alias":         {"rewake-linux-x64": "npm:" + packageName + "@1.0.0-linux-x64", scope + "/rewake-linux-arm64": "npm:" + packageName + "@1.0.0-linux-arm64"},
	} {
		wrong := entryGood
		wrong.OptionalDependencies = optional
		if problems := manifestProblems(entry, wrong, "1.0.0"); len(problems) != 1 || !strings.Contains(problems[0], "optionalDependencies") {
			t.Errorf("%s: %v", name, problems)
		}
	}
	withOS := entryGood
	withOS.OS = []string{"linux"}
	if problems := manifestProblems(entry, withOS, "1.0.0"); len(problems) != 1 || !strings.Contains(problems[0], "os or cpu") {
		t.Errorf("an entry with os: %v", problems)
	}
}

// A build's version is its release's with the platform as a prerelease
// suffix; its tag keeps it off latest, and a prerelease entry goes under next.
func TestVersionsAndTags(t *testing.T) {
	for _, tc := range []struct {
		p                pkg
		release, version string
		tag, ref         string
	}{
		{packages[0], "1.0.2", "1.0.2-linux-x64", "linux-x64", packageName + "@1.0.2-linux-x64"},
		{packages[1], "1.0.2", "1.0.2-linux-arm64", "linux-arm64", packageName + "@1.0.2-linux-arm64"},
		{packages[2], "1.0.2", "1.0.2", "latest", packageName + "@1.0.2"},
		{packages[0], "1.1.0-rc.1", "1.1.0-rc.1-linux-x64", "linux-x64", packageName + "@1.1.0-rc.1-linux-x64"},
		{packages[2], "1.1.0-rc.1", "1.1.0-rc.1", "next", packageName + "@1.1.0-rc.1"},
	} {
		if got := tc.p.version(tc.release); got != tc.version || !semver.MatchString(got) {
			t.Errorf("%s of %s: version %q, want %q and semver", tc.p.dir, tc.release, got, tc.version)
		}
		if got := tc.p.tag(tc.release); got != tc.tag {
			t.Errorf("%s of %s: tag %q, want %q", tc.p.dir, tc.release, got, tc.tag)
		}
		if got := tc.p.ref(tc.release); got != tc.ref {
			t.Errorf("%s of %s: ref %q, want %q", tc.p.dir, tc.release, got, tc.ref)
		}
	}
}

// The allowlist holds both ways: a missing file and an extra one are each a
// failure, and the command must be executable.
func TestFileProblems(t *testing.T) {
	entry := packages[2]
	files := func(paths ...string) packed {
		var p packed
		for _, path := range paths {
			mode := 0o644
			if path == "bin/rewake" {
				mode = 0o755
			}
			p.Files = append(p.Files, struct {
				Path string `json:"path"`
				Mode int    `json:"mode"`
			}{path, mode})
		}
		return p
	}
	if problems := fileProblems(entry, files("LICENSE", "README.md", "bin/rewake", "package.json")); len(problems) > 0 {
		t.Errorf("the exact set: %v", problems)
	}
	problems := fileProblems(entry, files("LICENSE", "bin/rewake", "package.json", ".env"))
	if len(problems) != 2 || !strings.Contains(problems[0], "README.md") || !strings.Contains(problems[1], ".env") {
		t.Errorf("one missing and one extra: %v", problems)
	}
	flat := files("LICENSE", "README.md", "bin/rewake", "package.json")
	flat.Files[2].Mode = 0o644
	if problems := fileProblems(entry, flat); len(problems) != 1 || !strings.Contains(problems[0], "not executable") {
		t.Errorf("a command that cannot run: %v", problems)
	}
}

// The test binary is an ELF of this machine: right for its own architecture,
// wrong for the other.
func TestCheckMachine(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	p, ok := platformFor(thisArch())
	if !ok {
		t.Skip("no package serves this machine")
	}
	if err := checkMachine(self, p.machine); err != nil {
		t.Errorf("this machine's binary: %v", err)
	}
	other := elf.EM_AARCH64
	if p.machine == elf.EM_AARCH64 {
		other = elf.EM_X86_64
	}
	if err := checkMachine(self, other); err == nil {
		t.Errorf("a binary of the other architecture passed")
	}
}
