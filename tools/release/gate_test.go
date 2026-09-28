package main

import (
	"context"
	"os"
	"path/filepath"
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

// viewed is the version an npm view line asks for, "" when it asks for none.
func viewed(line string) string {
	for _, word := range strings.Fields(line) {
		if version, ok := strings.CutPrefix(word, packageName+"@"); ok {
			return version
		}
	}
	return ""
}

// inRegistry answers npm view as the registry would once the release 1.0.0 is
// published and tagged: each version printed back, the dist-tags on it.
func inRegistry(line string) result {
	if strings.Contains(line, " dist-tags") {
		return result{stdout: `{"latest":"1.0.0","linux-x64":"1.0.0-linux-x64","linux-arm64":"1.0.0-linux-arm64"}`}
	}
	return result{stdout: `"` + viewed(line) + `"`}
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

// The registry is asked for each version of the release exactly, so a free
// entry does not hide a build that is already taken.
func TestTheRegistryIsAskedForEachVersion(t *testing.T) {
	f := &fake{answer: func(line string) result {
		if viewed(line) == "1.0.0-linux-arm64" {
			return inRegistry(line)
		}
		return e404
	}}
	g, out := newGate(t, call{version: "1.0.0"}, f)
	g.checkRegistry(context.Background())
	for _, version := range []string{"1.0.0-linux-x64", "1.0.0-linux-arm64", "1.0.0"} {
		if asked := f.called("npm view " + packageName + "@" + version + " version"); len(asked) != 1 {
			t.Errorf("asked for %s %d times, want once: %v", version, len(asked), f.calls)
		}
	}
	if len(g.failures) != 1 || !strings.Contains(g.failures[0], packageName+"@1.0.0-linux-arm64 is already in the registry") {
		t.Errorf("failures %v, want the taken build named\n%s", g.failures, out)
	}
}
