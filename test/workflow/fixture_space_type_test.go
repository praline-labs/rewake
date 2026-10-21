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

// TestFixtureSpaceType is a worker raised for a kind of work, on the fixture:
// a project's .rewake/aliases.toml names a template whose type is the
// project's own .rewake/types/survey.md; main, working in that project, opens
// the space without --type, and the worker's briefing carries the type's
// text. The type crosses from main's space open through the space host's
// environment into the worker's launch, which only a run of all three shows.
func TestFixtureSpaceType(t *testing.T) {
	binary := enterScenario(t, "fixture-space-type")
	c := Start(t, Spec{
		Name:         "fixture-space-type",
		Harness:      fixtureColumn.harness,
		Observations: fixtureSpaceTypeObservations,
		Deadline:     90 * time.Second,
	})
	iso := Isolate(t, c, binary)
	project := filepath.Join(iso.Home, "project")
	files := map[string]string{
		filepath.Join(project, ".rewake", "aliases.toml"):       fmt.Sprintf("[alias.worker]\nharness = %q\ntype = \"survey\"\n", fixtureColumn.harness),
		filepath.Join(project, ".rewake", "types", "survey.md"): "+++\ndescription = \"A survey of the code\"\n+++\n" + fixtureSpaceTypeText + "\n",
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
		c.Contradicted(obsSpaceTypeOpened, "main never became ready")
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
		Type    string `json:"type"`
	}
	if !answered || code != 0 || json.Unmarshal([]byte(out), &opened) != nil || opened.Session != worker.name || opened.Type != "survey" {
		c.Contradicted(obsSpaceTypeOpened, "space open answered %d, %v: %s", code, answered, out)
		return
	}
	c.Observed(obsSpaceTypeOpened, "space open took the project template's type: "+opened.Type)

	var text string
	briefed := waitFor(c, 20*time.Second, func() bool {
		data, err := os.ReadFile(brief)
		text = string(data)
		return err == nil && text != ""
	})
	if !briefed || !strings.Contains(text, "YOUR KIND OF WORK: survey") || !strings.Contains(text, fixtureSpaceTypeText) {
		c.Contradicted(obsSpaceTypeBriefed, "the worker's briefing (%v) is: %s", briefed, text)
		return
	}
	c.Observed(obsSpaceTypeBriefed, "the worker's briefing names the type and carries its text")

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
	obsSpaceTypeOpened   = "main's space open, in a project, takes the type its .rewake/aliases.toml template names from the project's .rewake/types/"
	obsSpaceTypeBriefed  = "the worker's briefing names that type and carries its text"
	fixtureSpaceTypeText = "fixture-space-type: read the module map first"
)

var fixtureSpaceTypeObservations = []string{obsSpaceTypeOpened, obsSpaceTypeBriefed}
