package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/taskbrief"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// projectMain makes "lead" main of a project whose .rewake/aliases.toml
// holds the text, and gives its record the project as its directory.
func projectMain(t *testing.T, aliases string) (string, registry.Session, string) {
	t.Helper()
	dir, worker := briefWorld(t)
	project := t.TempDir()
	if aliases != "" {
		if err := os.MkdirAll(filepath.Join(project, ".rewake"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(project, ".rewake", "aliases.toml"), []byte(aliases), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	lead, err := registry.Load(dir, "lead")
	if err != nil {
		t.Fatal(err)
	}
	lead.CWD = project
	raw, _ := json.MarshalIndent(lead, "", "  ")
	if err := os.WriteFile(state.SessionPath(dir, "lead"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, worker, project
}

// A brief goes where the project's setting says, a worker anywhere reads and
// comments it, and a comment is numbered among the comments.
func TestABriefInTheProjectsFolder(t *testing.T) {
	dir, worker, project := projectMain(t, "[briefs]\ndir = \"tasks\"\n")
	if code, out, errOut := run("brief", "new", "# Auth\n\n- [ ] one\n- [ ] two\n"); code != ExitOK || !strings.Contains(out, "brief 1 made") {
		t.Fatalf("new: %d %q %q", code, out, errOut)
	}
	if _, err := os.Stat(filepath.Join(project, "tasks", "brief-1", "brief.json")); err != nil {
		t.Fatalf("the brief is not in the project's folder: %v", err)
	}
	other := t.TempDir()
	if code, _, errOut := run("brief", "new", "--dir", other, "# Task\n"); code != ExitOK {
		t.Fatalf("new --dir: %d %q", code, errOut)
	}
	if _, err := os.Stat(filepath.Join(other, "brief-2", "brief.json")); err != nil {
		t.Fatalf("--dir did not win: %v", err)
	}
	if code, _, errOut := run("brief", "new", "--dir", filepath.Join(other, "missing"), "# Task\n"); code != ExitUsage || !strings.Contains(errOut, "not an existing directory") {
		t.Errorf("--dir that does not exist: %d %q", code, errOut)
	}
	if err := taskbrief.Bind(quietHomeOf(dir), 1, taskbrief.Author{Name: worker.Name, Epoch: worker.Epoch()}, taskbrief.Work); err != nil {
		t.Fatal(err)
	}
	as(t, worker)
	run("brief", "tick", "1", "1", "--edition", "1")
	run("brief", "comment", "1", "--", "first")
	code, out, errOut := run("brief", "comment", "1", "--", "second")
	if code != ExitOK || !strings.Contains(out, "comment #2 added") {
		t.Fatalf("a comment is numbered among comments, not the log's 4: %d %q %q", code, out, errOut)
	}
	code, out, errOut = run("brief", "show", "1", "--json")
	var shown struct {
		Comments []struct{ N int } `json:"comments"`
	}
	if code != ExitOK || json.Unmarshal([]byte(out), &shown) != nil || len(shown.Comments) != 2 || shown.Comments[0].N != 1 || shown.Comments[1].N != 2 {
		t.Fatalf("show: %d %q %q", code, out, errOut)
	}
}

// A project file that does not read refuses a new brief rather than make it
// in the room, where its folder-mates would not look.
func TestABriefIsNotMadeInTheWrongPlace(t *testing.T) {
	projectMain(t, "[briefs]\nfolder = \"tasks\"\n")
	if code, _, errOut := run("brief", "new", "# Auth\n"); code == ExitOK || !strings.Contains(errOut, "does not read") {
		t.Errorf("new with a bad project file: %d %q", code, errOut)
	}
}
