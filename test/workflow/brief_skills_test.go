package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestBriefGivesASkill is a brief's skill end to end (docs/briefs.md#skills):
// main makes a brief whose text names a skill of main's harness, rewake
// copies it into the brief, the worker sent the brief is given the copy's
// path in its letter and the file there holds the skill; main's close takes
// the copy away.
//
// The worker's own reading is its letter: the path it is given is the one the
// scenario reads, as the worker would.
func TestBriefGivesASkill(t *testing.T) {
	runInColumns(t, "brief-skill", func(t *testing.T, col column) {
		binary := enterScenario(t, "brief-skill")
		c := Start(t, Spec{
			Name:         "brief-skill",
			Harness:      col.harness,
			Observations: briefSkillObservations,
			Deadline:     90 * time.Second,
		})
		for _, finding := range playBriefSkill(t, c, Isolate(t, c, binary), col) {
			if finding.held {
				c.Observed(finding.observation, finding.detail)
			} else {
				c.Contradicted(finding.observation, "%s", finding.detail)
			}
		}
	})
}

const (
	obsSkillGiven  = "the worker's letter gives the path of the copied SKILL.md, which holds the skill main named"
	obsSkillClosed = "the brief's close removes the copy"

	skillProbeName = "brief-probe-skill"
	skillProbeBody = "brief-probe skill body: read me first"
	skillTaskText  = "brief-skill-probe: do the brief"
)

var briefSkillObservations = []string{obsSkillGiven, obsSkillClosed}

// skillPath finds a SKILL.md path in a letter's machine form.
var skillPath = regexp.MustCompile(`"path": "([^"]*/SKILL\.md)"`)

func playBriefSkill(t *testing.T, c *Case, iso *Isolation, col column) []telemetryFinding {
	t.Helper()
	unjudged := func(detail string) []telemetryFinding {
		var out []telemetryFinding
		for _, observation := range briefSkillObservations {
			out = append(out, telemetryFinding{observation: observation, detail: detail})
		}
		return out
	}
	// The fixture, run as main, reads its skills from its home directory.
	skill := filepath.Join(iso.Home, ".fixture", "skills", skillProbeName)
	if err := os.MkdirAll(skill, 0o700); err != nil {
		return unjudged(err.Error())
	}
	text := "---\nname: " + skillProbeName + "\ndescription: the probe's skill\n---\n\n" + skillProbeBody + "\n"
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte(text), 0o600); err != nil {
		return unjudged(err.Error())
	}

	works := newRequests(iso, "worker")
	worker := startHarnessSession(t, c, iso, col.harness, "worker", "--general", shimInboxJSON+"=1", works.env())
	defer stopSession(t, c, worker)
	asks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, col.harness, "lead", "--main", shimInboxJSON+"=1", asks.env())
	defer stopSession(t, c, lead)
	if err := sessionsUp(c, worker); err != nil {
		return unjudged(err.Error())
	}

	code, out, _ := asks.ask(c, "brief", "new", "--json", "--", "# Skill probe\n\nSkills: "+skillProbeName+"\n\n- [ ] read the skill\n")
	var made struct {
		ID int `json:"id"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &made) != nil || made.ID < 1 {
		return unjudged(fmt.Sprintf("main's brief new answered exit %d: %s", code, out))
	}
	id := fmt.Sprint(made.ID)
	if code, out, _ := asks.ask(c, "send", worker.name, skillTaskText, "--brief", id); code != 0 {
		return unjudged(fmt.Sprintf("main's send --brief answered exit %d: %s", code, out))
	}
	if !waitFor(c, 30*time.Second, func() bool {
		_, read := messageCarrying(worker, "brief-skill-probe")
		return read
	}) {
		return unjudged("the worker never read the task")
	}

	read := worker.mailboxRead()
	var path, body string
	if m := skillPath.FindStringSubmatch(read); m != nil {
		path = m[1]
		raw, _ := os.ReadFile(path)
		body = string(raw)
	}
	given := path != "" && strings.HasPrefix(path, iso.StateDir) && strings.Contains(path, "/"+skillProbeName+"/") &&
		strings.Contains(body, skillProbeBody)
	findings := []telemetryFinding{{
		observation: obsSkillGiven, held: given, judged: true,
		detail: fmt.Sprintf("the letter gave %q, holding %q; the worker's inbox read %q", path, body, read),
	}}

	closed, closeOut, _ := asks.ask(c, "brief", "close", id)
	_, err := os.Stat(path)
	gone := path != "" && closed == 0 && os.IsNotExist(err)
	findings = append(findings, telemetryFinding{
		observation: obsSkillClosed, held: gone, judged: true,
		detail: fmt.Sprintf("close exit %d: %s; the copy at %q: %v", closed, firstLine(closeOut), path, err),
	})
	return findings
}
