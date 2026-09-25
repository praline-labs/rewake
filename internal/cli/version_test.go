package cli

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

const stampedRevision = "36d6b90099994526d79ff0b8209340f420ebecb4"

func stamp(settings ...string) *debug.BuildInfo {
	info := &debug.BuildInfo{}
	for i := 0; i+1 < len(settings); i += 2 {
		info.Settings = append(info.Settings, debug.BuildSetting{Key: settings[i], Value: settings[i+1]})
	}
	return info
}

func TestBuildFromTheVCSStamp(t *testing.T) {
	committed := time.Date(2026, 9, 25, 11, 52, 28, 0, time.UTC)
	built := time.Date(2026, 9, 25, 11, 58, 3, 0, time.UTC)
	for name, tc := range map[string]struct {
		info    *debug.BuildInfo
		ok      bool
		builtAt string
		want    Build
	}{
		"built": {
			stamp("vcs", "git", "vcs.revision", stampedRevision, "vcs.time", "2026-09-25T11:52:28Z", "vcs.modified", "false"), true, "2026-09-25T11:58:03Z",
			Build{Version: Version, Known: true, Revision: stampedRevision, Built: &built, Committed: &committed},
		},
		"committed only": {
			stamp("vcs.revision", stampedRevision, "vcs.time", "2026-09-25T11:52:28Z", "vcs.modified", "true"), true, "",
			Build{Version: Version, Known: true, Revision: stampedRevision, Committed: &committed, Modified: true},
		},
		// A time in another zone is told in UTC, and one that does not
		// parse is none.
		"offset": {
			stamp("vcs.revision", stampedRevision, "vcs.time", "2026-09-25T14:52:28+03:00"), true, "yesterday",
			Build{Version: Version, Known: true, Revision: stampedRevision, Committed: &committed},
		},
		// go run and a build from an archive: settings, but no revision.
		"unstamped":       {stamp("-trimpath", "true", "vcs.time", "2026-09-25T11:52:28Z", "vcs.modified", "true"), true, "", Build{Version: Version}},
		"unstamped built": {stamp("-trimpath", "true"), true, "2026-09-25T14:58:03+03:00", Build{Version: Version, Built: &built}},
		"no info":         {nil, false, "", Build{Version: Version}},
		"no setting":      {stamp(), true, "", Build{Version: Version}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := buildFrom(tc.info, tc.ok, tc.builtAt); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("build %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestBuildLine(t *testing.T) {
	for _, tc := range []struct {
		build Build
		want  string
	}{
		{builtBuild(), "rewake 0.0.1 · build 36d6b90 · built 2026-09-25 11:58 UTC · modified"},
		{committedBuild(), "rewake 0.0.1 · build 36d6b90 · committed 2026-09-25 11:52 UTC"},
		{Build{Version: "0.0.1", Known: true, Revision: stampedRevision}, "rewake 0.0.1 · build 36d6b90"},
		{Build{Version: "0.0.1", Built: builtBuild().Built}, "rewake 0.0.1 · build unknown · built 2026-09-25 11:58 UTC"},
		{Build{Version: "0.0.1"}, "rewake 0.0.1 · build unknown"},
	} {
		if got := tc.build.Line(); got != tc.want {
			t.Errorf("line %q, want %q", got, tc.want)
		}
	}
}

// builtBuild is a build whose command passed its time in, from a modified
// tree; its commit time is known too, and the line prefers the build's.
func builtBuild() Build {
	built := time.Date(2026, 9, 25, 11, 58, 3, 0, time.UTC)
	build := committedBuild()
	build.Built, build.Modified = &built, true
	return build
}

// committedBuild is a plain go build in a clean checkout: the commit time
// only.
func committedBuild() Build {
	committed := time.Date(2026, 9, 25, 11, 52, 28, 0, time.UTC)
	return Build{Version: "0.0.1", Known: true, Revision: stampedRevision, Committed: &committed}
}

// injectBuild puts a fixed build in place of the test binary's own, which
// depends on how the tests were run.
func injectBuild(t *testing.T, build Build) {
	t.Helper()
	saved := thisBuild
	thisBuild = func() Build { return build }
	t.Cleanup(func() { thisBuild = saved })
}

// Each of the three forms, as --version prints it and in its machine form.
func TestVersionPrintsTheBuild(t *testing.T) {
	for _, tc := range []struct {
		build Build
		line  string
	}{
		{builtBuild(), "rewake 0.0.1 · build 36d6b90 · built 2026-09-25 11:58 UTC · modified"},
		{committedBuild(), "rewake 0.0.1 · build 36d6b90 · committed 2026-09-25 11:52 UTC"},
		{Build{Version: "0.0.1"}, "rewake 0.0.1 · build unknown"},
	} {
		injectBuild(t, tc.build)
		code, out, _ := run("--version")
		if code != ExitOK || out != tc.line+"\n" {
			t.Fatalf("exit %d, %q, want %q", code, out, tc.line)
		}
		code, out, _ = run("--version", "--json")
		var got Build
		if err := json.Unmarshal([]byte(out), &got); code != ExitOK || err != nil || !reflect.DeepEqual(got, tc.build) {
			t.Fatalf("exit %d, %v: %+v from %q", code, err, got, out)
		}
		// Each time is present only when known.
		for field, known := range map[string]bool{`"built"`: tc.build.Built != nil, `"committed"`: tc.build.Committed != nil} {
			if strings.Contains(out, field) != known {
				t.Fatalf("%s present %v in %s", field, !known, out)
			}
		}
	}
}

// The guide names its build before anything else, in both forms, so a session
// knows which binary answers it.
func TestTheGuideOpensWithTheBuild(t *testing.T) {
	build := builtBuild()
	injectBuild(t, build)
	for _, argv := range [][]string{{}, {"guide"}} {
		code, out, _ := run(argv...)
		if first, _, _ := strings.Cut(out, "\n"); code != ExitOK || first != build.Line() {
			t.Fatalf("%v: exit %d, first line %q", argv, code, first)
		}
		code, out, _ = run(append(argv, "--json")...)
		var model struct {
			Build Build `json:"build"`
		}
		if err := json.Unmarshal([]byte(out), &model); code != ExitOK || err != nil || !reflect.DeepEqual(model.Build, build) {
			t.Fatalf("%v --json: exit %d, %v: %+v", argv, code, err, model.Build)
		}
	}
}

// The build time reaches the binary by the flag the build commands pass:
// a variable renamed or moved would leave them setting nothing, silently.
// Without a VCS stamp: the test checks the time alone, and a checkout copied
// without its .git would fail the stamp.
func TestTheBuildTimeIsPassedIn(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "rewake")
	build := exec.Command("go", "build", "-buildvcs=false", "-ldflags", "-X github.com/iiiokojiadbi/rewake/internal/cli.built=2026-09-25T11:58:03Z", "-o", binary, "../../cmd/rewake")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	out, err := exec.Command(binary, "--version").Output()
	if err != nil || !strings.Contains(string(out), " · built 2026-09-25 11:58 UTC") {
		t.Fatalf("%v: %q", err, out)
	}
}
