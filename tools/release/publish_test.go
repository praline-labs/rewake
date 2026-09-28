package main

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// The platform builds go up before the entry, all public, with the code when
// one is given; the first failure stops the rest and names what was and was
// not published.
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
		"published:     " + packageName + "@1.0.0-linux-x64",
		"not published: " + packageName + "@1.0.0-linux-arm64, " + packageName + "@1.0.0",
		"1.0.0 cannot be reused",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q:\n%s", want, text)
		}
	}
}

// The platform builds go up before the entry that names them: an install
// landing between two uploads must never find an entry without its binary.
// Each goes under its own tag, since npm hands latest to an upload without
// one, and a build under latest is a package with no command in it.
func TestThePlatformsArePublishedFirstUnderTheirTags(t *testing.T) {
	for release, entryTag := range map[string]string{"1.0.0": "latest", "1.1.0-rc.1": "next"} {
		f := &fake{}
		g, out := newGate(t, call{version: release, publish: true}, f)
		if !g.publishAll(context.Background()) {
			t.Fatalf("uploads that succeeded reported failure\n%s", out)
		}
		if want := []string{"rewake-linux-x64", "rewake-linux-arm64", "rewake"}; !slices.Equal(f.published, want) {
			t.Errorf("published %v, want %v", f.published, want)
		}
		published := f.called("npm publish")
		for index, tag := range []string{"linux-x64", "linux-arm64", entryTag} {
			if index >= len(published) || !strings.Contains(published[index], " --tag "+tag+" ") {
				t.Errorf("%s: upload %d is not under --tag %s: %v", release, index, tag, published)
			}
		}
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

// Every version asked for by its whole number, and every tag on its upload:
// then the release is done.
func TestVerifyAsksForEachVersionAndTag(t *testing.T) {
	f := &fake{answer: inRegistry}
	g, out := newGate(t, call{version: "1.0.0", publish: true}, f)
	if !g.verify(context.Background()) {
		t.Fatalf("a release the registry shows whole did not count\n%s", out)
	}
	for _, version := range []string{"1.0.0-linux-x64", "1.0.0-linux-arm64", "1.0.0"} {
		if asked := f.called("npm view " + packageName + "@" + version + " version"); len(asked) != 1 {
			t.Errorf("asked for %s %d times, want once: %v", version, len(asked), f.calls)
		}
	}
	for _, want := range []string{"linux-x64 is 1.0.0-linux-x64", "linux-arm64 is 1.0.0-linux-arm64", "latest is 1.0.0"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

// Right after an upload the registry may not show it yet: reported, not
// retried, and no finished release.
func TestVerifyReportsWithoutRetrying(t *testing.T) {
	f := &fake{answer: func(line string) result {
		if viewed(line) == "1.0.0-linux-arm64" {
			return e404
		}
		return inRegistry(line)
	}}
	g, out := newGate(t, call{version: "1.0.0", publish: true}, f)
	if g.verify(context.Background()) {
		t.Errorf("a version not visible yet counted as seen")
	}
	if views := f.called("npm view"); len(views) != 4 {
		t.Errorf("asked %d times, want once per version and once for the tags: %v", len(views), views)
	}
	text := out.String()
	for _, want := range []string{"rewake@1.0.0-linux-x64 visible", "rewake@1.0.0-linux-arm64 not visible yet", "latest is 1.0.0"} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q:\n%s", want, text)
		}
	}
}

// A tag left on an older release, or missing, means the release is not what
// an install gets yet: no success, and the gate names the command that fixes
// it if it stays so.
func TestAWrongTagIsNoSuccess(t *testing.T) {
	for name, tags := range map[string]string{
		"latest behind":    `{"latest":"0.9.0","linux-x64":"1.0.0-linux-x64","linux-arm64":"1.0.0-linux-arm64"}`,
		"a build untagged": `{"latest":"1.0.0","linux-x64":"1.0.0-linux-x64"}`,
	} {
		f := &fake{answer: func(line string) result {
			if strings.Contains(line, " dist-tags") {
				return result{stdout: tags}
			}
			return inRegistry(line)
		}}
		g, out := newGate(t, call{version: "1.0.0", publish: true}, f)
		if g.verify(context.Background()) {
			t.Errorf("%s: counted as done\n%s", name, out)
		}
		want := "npm dist-tag add " + packageName + "@1.0.0 latest"
		if name == "a build untagged" {
			want = "npm dist-tag add " + packageName + "@1.0.0-linux-arm64 linux-arm64"
		}
		if !strings.Contains(out.String(), want) {
			t.Errorf("%s: output lacks %q:\n%s", name, want, out)
		}
	}
}

// A transport failure may come after the registry stored the upload: the gate
// asks once, and neither calls the version free nor gives up on an upload that
// went through.
func TestALostAnswerIsAskedAbout(t *testing.T) {
	for name, tc := range map[string]struct {
		view       func(string) result
		ok         bool
		uploads    int
		says       []string
		mustNotSay string
	}{
		"not there yet": {
			view: func(string) result { return e404 }, uploads: 1,
			says:       []string{"unknown:       " + packageName + "@1.0.0-linux-x64", "not published: " + packageName + "@1.0.0-linux-arm64, " + packageName + "@1.0.0", "do not reuse 1.0.0 until"},
			mustNotSay: "still free",
		},
		"registry unreachable": {
			view: func(string) result { return result{code: 1, stderr: "npm error code ETIMEDOUT\n"} }, uploads: 1,
			says:       []string{"unknown:       " + packageName + "@1.0.0-linux-x64", "do not reuse 1.0.0 until"},
			mustNotSay: "still free",
		},
		"stored after all": {
			view: inRegistry, ok: true, uploads: 3,
			says: []string{"went through, and the next one follows"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			uploads := 0
			f := &fake{answer: func(line string) result {
				if strings.HasPrefix(line, "npm view") {
					return tc.view(line)
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
		if viewed(line) == "1.0.0" {
			return e404
		}
		return inRegistry(line)
	}}
	g, out := newGate(t, call{version: "1.0.0", publish: true}, f)
	if g.verify(context.Background()) {
		t.Fatalf("an unseen package counted as seen\n%s", out)
	}
	for _, want := range []string{"it can also be an upload npm skipped", "npm view " + packageName + "@1.0.0 version", "tag only once each is there"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}
