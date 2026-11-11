package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter/pi"
	"github.com/praline-labs/rewake/internal/infra/parts"
)

func (r *repo) installPi(version string) {
	r.t.Helper()
	path := filepath.Join(r.install, "node_modules", pi.Package, "package.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"version":"`+version+`"}`), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r *repo) npmCalls() []string {
	var calls []string
	for _, call := range r.calls {
		if strings.HasPrefix(call, "npm ") {
			calls = append(calls, call)
		}
	}
	return calls
}

// The live pin update installed the binary but left the old pi beside it.
// Land must install a missing or different version before recording it.
func TestLandInstallsThePinnedPi(t *testing.T) {
	for _, version := range []string{"1.4.0", "", pi.PinnedVersion} {
		name := version
		if name == "" {
			name = "absent"
		}
		t.Run(name, func(t *testing.T) {
			r := newRepo(t)
			if version != "" {
				r.installPi(version)
			}
			r.branchOff("work", "a.go", "package a\n")
			code, out := r.land("work")
			if code != exitDone {
				t.Fatalf("exit %d\n%s", code, out)
			}
			var want []string
			if version != pi.PinnedVersion {
				want = []string{"npm install --prefix " + r.install + " --save-exact " + pi.Package + "@" + pi.PinnedVersion}
			}
			if calls := r.npmCalls(); !slices.Equal(calls, want) {
				t.Errorf("npm calls %v, want %v", calls, want)
			}
			if got := installedPi(r.install); got != pi.PinnedVersion {
				t.Errorf("installed pi %q, want %q", got, pi.PinnedVersion)
			}
			m, err := parts.Read(r.install)
			if err != nil {
				t.Fatal(err)
			}
			part, found := m.Find("pi")
			if !found || part.Digest != "npm:"+pi.Package+"@"+pi.PinnedVersion || part.Speaks["pi"] != pi.PinnedVersion {
				t.Errorf("manifest pi %+v, want the pin %s", part, pi.PinnedVersion)
			}
		})
	}
}

// Land is compiled before it merges the target: the bundled package must
// follow the landed sources, not the pin compiled into the land process.
func TestLandUsesTheLandedPiPin(t *testing.T) {
	r := newRepo(t)
	r.installPi(pi.PinnedVersion)
	r.piPin = "1.5.3"
	if r.piPin == pi.PinnedVersion {
		t.Fatal("the fixture's landed pin must differ from the land process's pin")
	}
	r.branchOff("pin", "internal/adapter/pi/ship.go", "package pi\nconst (\n\tPinnedVersion = \""+r.piPin+"\"\n)\n")
	code, out := r.land("pin")
	if code != exitDone {
		t.Fatalf("exit %d\n%s", code, out)
	}
	want := []string{"npm install --prefix " + r.install + " --save-exact " + pi.Package + "@" + r.piPin}
	if calls := r.npmCalls(); !slices.Equal(calls, want) {
		t.Errorf("npm calls %v, want %v", calls, want)
	}
	if got := installedPi(r.install); got != r.piPin {
		t.Errorf("installed pi %q, want landed pin %q", got, r.piPin)
	}
	m, err := parts.Read(r.install)
	if err != nil {
		t.Fatal(err)
	}
	part, _ := m.Find("pi")
	rewake, _ := m.Find(parts.Rewake)
	if part.Speaks["pi"] != r.piPin || rewake.Requires["pi"] != r.piPin {
		t.Errorf("manifest pi %+v and rewake %+v disagree with landed pin %s", part, rewake, r.piPin)
	}
}

// An npm failure or an unsuccessful update is an install failure, not a
// green landing with a manifest describing the old package as the new one.
func TestLandRefusesAnUninstalledPiPin(t *testing.T) {
	for _, failure := range []struct{ red, why string }{
		{"npm", "npm install failed"},
		{"pi-version", `pi "1.4.0" is installed after npm install; want ` + pi.PinnedVersion},
	} {
		t.Run(failure.red, func(t *testing.T) {
			r := newRepo(t)
			r.branchOff("first", "a.go", "package a\n")
			if code, out := r.land("first"); code != exitDone {
				t.Fatalf("exit %d\n%s", code, out)
			}
			if _, err := parts.Read(r.install); err != nil {
				t.Fatal(err)
			}
			r.installPi("1.4.0")
			r.red[failure.red] = true
			r.calls = nil
			tip := r.branchOff("work", "b.go", "package b\n")
			code, out := r.land("work")
			if code != exitFailed || !strings.Contains(out, "FAIL  install:") || !strings.Contains(out, failure.why) || !strings.Contains(out, "v2 is pushed, the room's build is the old one") {
				t.Errorf("exit %d, want install failure with %q\n%s", code, failure.why, out)
			}
			if r.at(branch) != tip || r.git(r.origin, "rev-parse", branch) != tip {
				t.Error("a failed install took back the pushed landing")
			}
			if len(r.npmCalls()) != 1 || r.built() {
				t.Errorf("want one npm call and no binary replacement: %v", r.calls)
			}
			if _, err := os.Stat(filepath.Join(r.install, parts.FileName)); !os.IsNotExist(err) {
				t.Errorf("a misleading manifest remains: %v", err)
			}
		})
	}
}
