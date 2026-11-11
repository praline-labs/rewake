package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestFixtureSpaceMode is a worker raised in a runtime work mode, on the
// fixture: a project's .rewake/aliases.toml names a template whose start_mode
// is the project's own .rewake/modes/survey.md; main, working in that project,
// opens the space without --work-mode, and the worker's briefing carries that
// one mode section. The mode crosses from main's space open through the space
// host's environment into the worker's launch, which only a run of all three
// shows.
func TestFixtureSpaceMode(t *testing.T) {
	binary := enterScenario(t, "fixture-space-mode")
	c := Start(t, Spec{
		Name:         "fixture-space-mode",
		Harness:      fixtureColumn.harness,
		Observations: fixtureSpaceModeObservations,
		Deadline:     90 * time.Second,
	})
	iso := Isolate(t, c, binary)
	project := filepath.Join(iso.Home, "project")
	files := map[string]string{
		filepath.Join(project, ".rewake", "aliases.toml"):       fmt.Sprintf("[alias.worker]\nharness = %q\n[alias.worker.pi]\nstart_mode = \"survey\"\n", fixtureColumn.harness),
		filepath.Join(project, ".rewake", "modes", "survey.md"): "+++\ndescription = \"A survey of the code\"\n+++\n" + fixtureSpaceModeText + "\n",
	}
	for path, text := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	asks := newRequests(iso, "lead")
	lead := startSessionIn(t, c, iso, project, fixtureColumn.harness, "lead", "--main", nil, nil, asks.env(), shimInboxJSON+"=1")
	defer stopSession(t, c, lead)
	if !waitFor(c, 20*time.Second, func() bool { _, err := os.Stat(lead.ready); return err == nil }) {
		c.Contradicted(obsSpaceModeOpened, "main never became ready")
		return
	}
	worker, env := spaceWorker(iso, "survey", fixtureColumn.harness)
	brief := filepath.Join(iso.Home, "survey.brief")
	env = append(env, shimBriefFile+"="+brief)
	encoded, _ := json.Marshal(shimRequest{Args: []string{"space", "open", "survey", "--alias", "worker", "--json"}, Env: env})
	code, out, answered := asks.send(c, encoded, "space open survey")
	var opened struct {
		Session string `json:"session"`
		ID      string `json:"id"`
		Mode    string `json:"mode"`
	}
	if !answered || code != 0 || json.Unmarshal([]byte(out), &opened) != nil || opened.Session != worker.name || opened.Mode != "survey" {
		c.Contradicted(obsSpaceModeOpened, "space open answered %d, %v: %s", code, answered, out)
		return
	}
	c.Observed(obsSpaceModeOpened, "space open took the project template's mode: "+opened.Mode)

	var text string
	briefed := waitFor(c, 20*time.Second, func() bool {
		data, err := os.ReadFile(brief)
		text = string(data)
		return err == nil && text != ""
	})
	if !briefed || !strings.Contains(text, "Runtime work mode: survey.") || !strings.Contains(text, fixtureSpaceModeText) {
		c.Contradicted(obsSpaceModeBriefed, "the worker's briefing (%v) is: %s", briefed, text)
		return
	}
	c.Observed(obsSpaceModeBriefed, "the worker's briefing names the mode and carries its one section")

	// The case leaves nothing running: the worker and its space host end
	// with the close.
	if code, out, _ := asks.ask(c, "space", "close", "survey"); code != 0 {
		t.Errorf("space close answered %d: %s", code, out)
	}
	waitFor(c, 20*time.Second, func() bool {
		record := spaceRecordOf(iso, opened.ID)
		return record.State == "closed" && !runAlive(record.Worker.Run) && !runAlive(record.Host)
	})
}

const (
	obsSpaceModeOpened   = "main's space open, in a project, takes the mode its .rewake/aliases.toml template names from the project's .rewake/modes/"
	obsSpaceModeBriefed  = "the worker's briefing names that mode and carries its one section"
	fixtureSpaceModeText = "fixture-space-mode: read the module map first"
)

var fixtureSpaceModeObservations = []string{obsSpaceModeOpened, obsSpaceModeBriefed}
