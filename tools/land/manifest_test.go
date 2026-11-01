package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/parts"
)

// shellSays is rewake-shell --contracts for a shell requiring host and face.
func shellSays(host, face string) string {
	return `{"shell":{"requires":{"host":"` + host + `","face":"` + face + `"}},"face":{"speaks":{"face":"` + face + `"},"requires":{"mirror":"1"}}}`
}

// goFiles stands for the digest of rewake's sources: the test's checkout
// has no module to list, so its .go files are rewake's.
func goFiles(_ context.Context, root string) (string, error) {
	sum := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		data, err := os.ReadFile(path)
		sum.Write([]byte(path))
		sum.Write(data)
		return err
	})
	return hex.EncodeToString(sum.Sum(nil)), err
}

func (r *repo) built() bool {
	return slices.ContainsFunc(r.calls, func(c string) bool { return strings.HasPrefix(c, "go build") })
}

// Each landing says what the room must restart to run it, from the manifest
// before it to the one it writes; rewake is built only when its sources
// changed, so a landing of the shell alone asks for no room restart.
func TestLandSaysWhatToRestart(t *testing.T) {
	r := newRepo(t)
	steps := []struct {
		name, file string
		contracts  string
		build      bool
		want       string
	}{
		{"first", "a.go", "", true, "restart: room (no manifest before this landing)"},
		{"shell", "shell/app.ts", "", false, "restart: shell (rewake restart shell; shell changed)"},
		{"face", "face/src/worker.ts", "", false, "restart: nothing (the face reloads from its files)"},
		{"go", "b.go", "", true, "restart: room (rewake changed)"},
		{"facecontract", "face/src/messages.ts", shellSays("1", "6"), false, "restart: shell (rewake restart shell; shell: it requires face 5, and the installed face speaks 6)"},
		{"nothing", "docs.md", "", false, "restart: nothing"},
	}
	for i, step := range steps {
		r.calls = nil
		if step.contracts != "" {
			r.shellContracts = step.contracts
		}
		r.branchOff("work"+step.name, step.file, step.name+"\n")
		code, out := r.land("work" + step.name)
		if code != exitDone {
			t.Fatalf("%d %s: exit %d\n%s", i, step.name, code, out)
		}
		if !strings.Contains(out, "\n"+step.want+"\n") {
			t.Errorf("%s: want the line %q:\n%s", step.name, step.want, out)
		}
		if r.built() != step.build {
			t.Errorf("%s: built %v, want %v: %v", step.name, r.built(), step.build, r.calls)
		}
		m, err := parts.Read(r.install)
		if err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		rewake, _ := m.Find(parts.Rewake)
		if file, _ := parts.FileDigest(filepath.Join(r.install, "rewake")); rewake.File != file || rewake.Digest == "" {
			t.Errorf("%s: the manifest's rewake %+v is not the installed build %s", step.name, rewake, file)
		}
	}
}

// A landing whose manifest cannot be made still installs the build, and
// leaves no manifest of the build before it to mislead the room's plan.
func TestLandWithoutAManifestNeedsTheRoom(t *testing.T) {
	r := newRepo(t)
	if err := os.WriteFile(filepath.Join(r.install, parts.FileName), []byte(`{"manifest":1,"parts":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	r.shellContracts = "not json"
	r.branchOff("work", "a.go", "package a\n")
	code, out := r.land("work")
	if code != exitDone || !strings.Contains(out, "restart: room (no manifest: it could not be made)") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(r.install, parts.FileName)); !os.IsNotExist(err) {
		t.Errorf("the old manifest stayed: %v", err)
	}
	if build, _ := os.ReadFile(filepath.Join(r.install, "rewake")); string(build) != "new build" {
		t.Errorf("the install holds %q", build)
	}
}

// A build put in place by hand is not the one the manifest names, though
// rewake's sources are: land builds again (found in review).
func TestLandReplacesABuildPutInPlaceByHand(t *testing.T) {
	r := newRepo(t)
	r.branchOff("work", "a.go", "package a\n")
	if code, out := r.land("work"); code != exitDone {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if err := os.WriteFile(filepath.Join(r.install, "rewake"), []byte("by hand"), 0o755); err != nil {
		t.Fatal(err)
	}
	r.calls = nil
	r.branchOff("docs", "docs.md", "docs\n")
	code, out := r.land("docs")
	if code != exitDone || !r.built() || strings.Contains(out, "skip  build") {
		t.Fatalf("exit %d, built %v\n%s", code, r.built(), out)
	}
}
