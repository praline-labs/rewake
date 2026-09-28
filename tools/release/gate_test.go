package main

import (
	"context"
	"debug/elf"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// fake stands in for git, npm and the build: each call is recorded, and the
// answer comes from the test's function.
type fake struct {
	calls []string
	// published is the directory of each npm publish, in order.
	published []string
	answer    func(line string) result
}

func (f *fake) run(_ context.Context, _ time.Duration, dir string, name string, args ...string) result {
	line := strings.Join(append([]string{filepath.Base(name)}, args...), " ")
	f.calls = append(f.calls, line)
	if strings.HasPrefix(line, "npm publish") {
		f.published = append(f.published, filepath.Base(dir))
	}
	if f.answer != nil {
		return f.answer(line)
	}
	return result{}
}

func (f *fake) called(prefix string) []string {
	var matched []string
	for _, line := range f.calls {
		if strings.HasPrefix(line, prefix) {
			matched = append(matched, line)
		}
	}
	return matched
}

// newGate is a gate over a checkout that has a pack.sh and nothing else.
func newGate(t *testing.T, c call, f *fake) (*gate, *strings.Builder) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", "pack.sh"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	return &gate{call: c, run: f.run, out: &out, root: root, arch: "x64"}, &out
}

// E404 is npm's answer for a version the registry does not have.
var e404 = result{code: 1, stdout: `{"error":{"code":"E404","summary":"Not Found"}}`}

// cleanRepository answers git as a clean, pushed checkout would.
func cleanRepository(line string) (result, bool) {
	switch {
	case strings.HasPrefix(line, "git status"):
		return result{}, true
	case strings.HasPrefix(line, "git rev-parse HEAD"):
		return result{stdout: "0123456789abcdef0123456789abcdef01234567\n"}, true
	case strings.HasPrefix(line, "git merge-base"):
		return result{}, true
	case strings.HasPrefix(line, "git show HEAD:"+versionFile):
		return result{stdout: "package cli\n\nvar Version = \"1.0.0\"\n"}, true
	}
	return result{}, false
}

// With --publish, a failed check before the build stops the release there:
// nothing is built, nothing is uploaded.
func TestAFailedPreconditionStopsAPublish(t *testing.T) {
	for name, answer := range map[string]func(string) result{
		"dirty tree": func(line string) result {
			if strings.HasPrefix(line, "git status") {
				return result{stdout: " M README.md\n"}
			}
			if r, ok := cleanRepository(line); ok {
				return r
			}
			return e404
		},
		"not pushed": func(line string) result {
			if strings.HasPrefix(line, "git merge-base") {
				return result{code: 1}
			}
			if r, ok := cleanRepository(line); ok {
				return r
			}
			return e404
		},
		"source version differs": func(line string) result {
			if strings.HasPrefix(line, "git show") {
				return result{stdout: "var Version = \"0.0.1\"\n"}
			}
			if r, ok := cleanRepository(line); ok {
				return r
			}
			return e404
		},
		"registry unreachable": func(line string) result {
			if r, ok := cleanRepository(line); ok {
				return r
			}
			return result{code: 1, stderr: "npm error code ECONNRESET\nnpm error network aborted\n"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := &fake{answer: answer}
			g, out := newGate(t, call{version: "1.0.0", publish: true}, f)
			if code := g.release(context.Background()); code != exitFailed {
				t.Fatalf("exit %d, want %d\n%s", code, exitFailed, out)
			}
			if built, published := f.called("sh"), f.called("npm publish"); len(built) > 0 || len(published) > 0 {
				t.Fatalf("built %v, published %v\n%s", built, published, out)
			}
			if !strings.Contains(out.String(), "nothing was built or published") {
				t.Errorf("the refusal does not say nothing happened:\n%s", out)
			}
		})
	}
}

// A dry run goes on past a failed check, to show every problem in one run,
// and never reaches npm publish.
func TestADryRunNeverPublishes(t *testing.T) {
	f := &fake{answer: func(line string) result {
		if strings.HasPrefix(line, "sh ") {
			return result{}
		}
		if strings.HasPrefix(line, "git status") {
			return result{stdout: " M README.md\n"}
		}
		if r, ok := cleanRepository(line); ok {
			return r
		}
		return e404
	}}
	g, out := newGate(t, call{version: "1.0.0"}, f)
	if code := g.release(context.Background()); code != exitFailed {
		t.Fatalf("exit %d, want %d\n%s", code, exitFailed, out)
	}
	if len(f.called("sh")) != 1 {
		t.Errorf("a dry run with a dirty tree did not build: %v", f.calls)
	}
	if published := f.called("npm publish"); len(published) > 0 {
		t.Fatalf("a dry run published: %v", published)
	}
	if !strings.Contains(out.String(), "dry run: nothing was published") {
		t.Errorf("the dry run does not say so:\n%s", out)
	}
}

// The platforms go up before the entry, all public, with the code when one is
// given; the first failure stops the rest and names what was and was not
// published.
func TestPublishingStopsAtTheFirstFailure(t *testing.T) {
	uploads := 0
	f := &fake{answer: func(line string) result {
		if strings.HasPrefix(line, "npm publish") {
			uploads++
			if uploads == 2 {
				return result{code: 1, stderr: "npm error code E403\nnpm error 403 Forbidden\n"}
			}
		}
		return result{}
	}}
	g, out := newGate(t, call{version: "1.0.0", publish: true, otp: "123456"}, f)
	if g.publishAll(context.Background()) {
		t.Fatalf("a failed upload reported success\n%s", out)
	}
	published := f.called("npm publish")
	if len(published) != 2 {
		t.Fatalf("published %v, want two attempts and a stop", published)
	}
	for _, line := range published {
		for _, flag := range []string{"--dry-run=false", "--access public", "--otp 123456", "--registry " + registry} {
			if !strings.Contains(line, flag) {
				t.Errorf("%q lacks %s", line, flag)
			}
		}
	}
	text := out.String()
	for _, want := range []string{
		"published:     " + scope + "/rewake-linux-x64@1.0.0",
		"not published: " + scope + "/rewake-linux-arm64@1.0.0, " + scope + "/rewake@1.0.0",
		"1.0.0 cannot be reused",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q:\n%s", want, text)
		}
	}
}

// The platforms go up before the entry that names them: an install landing
// between two uploads must never find an entry without its binary.
func TestThePlatformsArePublishedFirst(t *testing.T) {
	f := &fake{}
	g, out := newGate(t, call{version: "1.0.0", publish: true}, f)
	if !g.publishAll(context.Background()) {
		t.Fatalf("uploads that succeeded reported failure\n%s", out)
	}
	if want := []string{"rewake-linux-x64", "rewake-linux-arm64", "rewake"}; !slices.Equal(f.published, want) {
		t.Errorf("published %v, want %v", f.published, want)
	}
}

// A first upload that fails leaves the version free, and the gate says so
// rather than sending the owner to the next version for nothing.
func TestAFirstFailedUploadLeavesTheVersionFree(t *testing.T) {
	f := &fake{answer: func(line string) result {
		if strings.HasPrefix(line, "npm publish") {
			return result{code: 1, stderr: "npm error code EOTP\n"}
		}
		return result{}
	}}
	g, out := newGate(t, call{version: "1.0.0", publish: true}, f)
	if g.publishAll(context.Background()) {
		t.Fatal("a failed upload reported success")
	}
	if !strings.Contains(out.String(), "published:     none") || !strings.Contains(out.String(), "1.0.0 is still free") {
		t.Errorf("output:\n%s", out)
	}
}

// Right after an upload the registry may not show it yet: reported, not
// retried and not a failure.
func TestVerifyReportsWithoutRetrying(t *testing.T) {
	f := &fake{answer: func(line string) result {
		if strings.Contains(line, "dist-tags.latest") {
			return result{stdout: `"1.0.0"`}
		}
		if strings.Contains(line, "rewake-linux-arm64") {
			return e404
		}
		return result{stdout: `"1.0.0"`}
	}}
	g, out := newGate(t, call{version: "1.0.0", publish: true}, f)
	if g.verify(context.Background()) {
		t.Errorf("a package not visible yet counted as seen")
	}
	if views := f.called("npm view"); len(views) != 4 {
		t.Errorf("asked %d times, want once per package and once for latest: %v", len(views), views)
	}
	text := out.String()
	for _, want := range []string{"rewake-linux-x64@1.0.0 visible", "rewake-linux-arm64@1.0.0 not visible yet", "latest is 1.0.0"} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q:\n%s", want, text)
		}
	}
}

func TestManifestProblems(t *testing.T) {
	x64, entry := packages[0], packages[2]
	good := manifest{Name: x64.name, Version: "1.0.0", License: "MIT", OS: []string{"linux"}, CPU: []string{"x64"}}
	good.Repository.URL = repository
	if problems := manifestProblems(x64, good, "1.0.0"); len(problems) > 0 {
		t.Errorf("a right platform manifest: %v", problems)
	}
	wrong := good
	wrong.CPU = []string{"arm64"}
	if problems := manifestProblems(x64, wrong, "1.0.0"); len(problems) != 1 || !strings.Contains(problems[0], "cpu") {
		t.Errorf("a wrong cpu: %v", problems)
	}
	entryGood := manifest{
		Name: entry.name, Version: "1.0.0", License: "MIT", Bin: map[string]string{"rewake": "bin/rewake"},
		OptionalDependencies: map[string]string{scope + "/rewake-linux-x64": "1.0.0", scope + "/rewake-linux-arm64": "1.0.0"},
	}
	entryGood.Repository.URL = repository
	if problems := manifestProblems(entry, entryGood, "1.0.0"); len(problems) > 0 {
		t.Errorf("a right entry manifest: %v", problems)
	}
	entryWrong := entryGood
	entryWrong.OptionalDependencies = map[string]string{scope + "/rewake-linux-x64": "^1.0.0"}
	entryWrong.OS = []string{"linux"}
	if problems := manifestProblems(entry, entryWrong, "1.0.0"); len(problems) != 2 {
		t.Errorf("an entry with os and a loose dependency: %v", problems)
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

// The committed default version must be the released one, and the refusal
// names the file and the line to write.
func TestTheSourceVersionMustMatch(t *testing.T) {
	for committed, want := range map[string]string{
		`var Version = "1.0.0"`:   "ok    " + versionFile + ` at HEAD says Version = "1.0.0"`,
		`var Version = "0.0.1"`:   `set it to var Version = "1.0.0" in the release commit`,
		`const Version = "1.0.0"`: "has no line",
	} {
		f := &fake{answer: func(string) result { return result{stdout: "package cli\n\n" + committed + "\n"} }}
		g, out := newGate(t, call{version: "1.0.0"}, f)
		g.checkSourceVersion(context.Background())
		if !strings.Contains(out.String(), want) {
			t.Errorf("%s: output lacks %q:\n%s", committed, want, out)
		}
		if shown := f.called("git show HEAD:" + versionFile); len(shown) != 1 {
			t.Errorf("read %v, want the file at HEAD", f.calls)
		}
	}
}

// A transport failure may come after the registry stored the upload: the gate
// asks once, and neither calls the version free nor gives up on a package that
// went through.
func TestALostAnswerIsAskedAbout(t *testing.T) {
	for name, tc := range map[string]struct {
		view       result
		ok         bool
		uploads    int
		says       []string
		mustNotSay string
	}{
		"not there yet": {
			view: e404, uploads: 1,
			says:       []string{"unknown:       " + scope + "/rewake-linux-x64@1.0.0", "not published: " + scope + "/rewake-linux-arm64@1.0.0, " + scope + "/rewake@1.0.0", "do not reuse 1.0.0 until"},
			mustNotSay: "still free",
		},
		"registry unreachable": {
			view: result{code: 1, stderr: "npm error code ETIMEDOUT\n"}, uploads: 1,
			says:       []string{"unknown:       " + scope + "/rewake-linux-x64@1.0.0", "do not reuse 1.0.0 until"},
			mustNotSay: "still free",
		},
		"stored after all": {
			view: result{stdout: `"1.0.0"`}, ok: true, uploads: 3,
			says: []string{"went through, and the next one follows"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			uploads := 0
			f := &fake{answer: func(line string) result {
				if strings.HasPrefix(line, "npm view") {
					return tc.view
				}
				uploads++
				if uploads == 1 {
					return result{code: 1, stderr: "npm error code ECONNRESET\nnpm error network socket hang up\n"}
				}
				return result{}
			}}
			g, out := newGate(t, call{version: "1.0.0", publish: true}, f)
			if got := g.publishAll(context.Background()); got != tc.ok {
				t.Fatalf("publishAll = %v, want %v\n%s", got, tc.ok, out)
			}
			if uploads != tc.uploads {
				t.Errorf("%d uploads, want %d\n%s", uploads, tc.uploads, out)
			}
			for _, want := range tc.says {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output lacks %q:\n%s", want, out)
				}
			}
			if tc.mustNotSay != "" && strings.Contains(out.String(), tc.mustNotSay) {
				t.Errorf("output says %q:\n%s", tc.mustNotSay, out)
			}
		})
	}
}

// npm said every upload went through, but the registry does not show one: that
// is no finished release, and the gate says what it may mean.
func TestAnUnseenPackageIsNoSuccess(t *testing.T) {
	f := &fake{answer: func(line string) result {
		if strings.Contains(line, "rewake@1.0.0") {
			return e404
		}
		return result{stdout: `"1.0.0"`}
	}}
	g, out := newGate(t, call{version: "1.0.0", publish: true}, f)
	if g.verify(context.Background()) {
		t.Fatalf("an unseen package counted as seen\n%s", out)
	}
	for _, want := range []string{"it can also be an upload npm skipped", "npm view " + scope + "/rewake@1.0.0 version", "tag only once each is there"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}
