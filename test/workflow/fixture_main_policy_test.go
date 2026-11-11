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

// TestFixtureMainPolicy is an opted-in main on the fixture: its project sets
// [main] implements = true and defines only a general reviewer. Startup, guide
// and workers agree that main may implement; main still delegates a review and
// reads the report; editing the project file afterwards changes neither the
// live main's guidance nor its listing; and a fresh main in that project takes
// the changed value. The project's other side — no writer template — is
// inventory, not an authority decision.
func TestFixtureMainPolicy(t *testing.T) {
	binary := enterScenario(t, "fixture-main-policy")
	c := Start(t, Spec{
		Name:         "fixture-main-policy",
		Harness:      fixtureColumn.harness,
		Observations: fixtureMainPolicyObservations,
		Deadline:     120 * time.Second,
	})
	iso := Isolate(t, c, binary)
	project := filepath.Join(iso.Home, "project")
	writeMainPolicyProject(t, project, true)
	brief := filepath.Join(iso.Home, "lead.brief")
	asks := newRequests(iso, "lead")
	lead := startSessionIn(t, c, iso, project, fixtureColumn.harness, "lead", "--main", nil, nil,
		asks.env(), shimInboxJSON+"=1", shimBriefFile+"="+brief)
	defer stopSession(t, c, lead)
	if !waitFor(c, 20*time.Second, func() bool { _, err := os.Stat(lead.ready); return err == nil }) {
		c.Contradicted(obsPolicyStartup, "main never became ready")
		return
	}
	text := ""
	if !waitFor(c, 20*time.Second, func() bool {
		raw, err := os.ReadFile(brief)
		text = string(raw)
		return err == nil && text != ""
	}) {
		c.Contradicted(obsPolicyStartup, "main received no briefing")
		return
	}
	if !strings.Contains(text, "lets main implement product changes itself") ||
		!strings.Contains(text, "may implement product changes yourself") ||
		strings.Contains(text, "Do not write product code yourself") {
		c.Contradicted(obsPolicyStartup, "the briefing: %s", text)
		return
	}
	c.Observed(obsPolicyStartup, "main's launch briefing states the opting-in policy and carries the implementing rule")

	// Guide and workers, from main's own shell: one policy everywhere.
	if code, out, answered := asks.ask(c, "guide"); !answered || code != 0 ||
		!strings.Contains(out, "may implement product changes yourself") || strings.Contains(out, "Do not write product code yourself") {
		c.Contradicted(obsPolicyGuideWorkers, "guide answered %d, %v: %s", code, answered, out)
		return
	}
	listing := workersOf(t, c, asks)
	if !listing.MainImplements || listing.MainPolicySource != "session" || !listing.NoWriter ||
		len(listing.Workers) != 1 || listing.Workers[0].Name != "reviewer" {
		c.Contradicted(obsPolicyGuideWorkers, "workers: %+v", listing)
		return
	}
	c.Observed(obsPolicyGuideWorkers, "guide and workers both say main may implement, and noWriter stays inventory")

	// A review is delegated and its report read: main implements is a policy,
	// not a refusal to coordinate.
	worker, env := spaceWorker(iso, "review", fixtureColumn.harness)
	// The worker reads the same project file, and must not draw main's policy
	// from it: its own briefing is the worker's.
	reviewerBrief := filepath.Join(iso.Home, "review.brief")
	env = append(env, shimBriefFile+"="+reviewerBrief)
	encoded, _ := json.Marshal(shimRequest{Args: []string{"space", "open", "review", "--alias", "reviewer", "--work-mode", "review", "--json"}, Env: env})
	code, out, answered := asks.send(c, encoded, "space open review")
	var opened struct {
		Session string `json:"session"`
	}
	if !answered || code != 0 || json.Unmarshal([]byte(out), &opened) != nil || opened.Session != worker.name {
		c.Contradicted(obsPolicyReviewDelegated, "space open answered %d, %v: %s", code, answered, out)
		return
	}
	if code, out, _ := asks.ask(c, "send", worker.name, fixtureMainPolicyReview); code != 0 {
		c.Contradicted(obsPolicyReviewDelegated, "the review was not sent: %d %s", code, out)
		return
	}
	c.Await("the reviewer to read the task", func() bool { return strings.Contains(worker.mailboxRead(), fixtureMainPolicyReview) })
	var report reportView
	found := false
	c.Await("main to read the reviewer's report", func() bool {
		report, found = reportOfKind(lead, worker, "finished")
		return found
	})
	if !found {
		c.Contradicted(obsPolicyReviewDelegated, "no finished report from %s reached main", worker.name)
		return
	}
	workerText := ""
	if !waitFor(c, 20*time.Second, func() bool {
		raw, err := os.ReadFile(reviewerBrief)
		workerText = string(raw)
		return err == nil && workerText != ""
	}) || strings.Contains(workerText, "Main implementation") || strings.Contains(workerText, "may implement product changes yourself") {
		c.Contradicted(obsPolicyReviewDelegated, "the worker's briefing took main's policy: %s", workerText)
		return
	}
	c.Observed(obsPolicyReviewDelegated, "the reviewer read the task, main read its finished report "+strings.Join(report.InReplyTo, " ")+", and the worker's own briefing carried no main policy")

	// An edit of the project file after the launch changes neither the live
	// main's guidance nor the listing, which reads that main's record.
	writeMainPolicyProject(t, project, false)
	if code, out, answered := asks.ask(c, "guide"); !answered || code != 0 ||
		!strings.Contains(out, "may implement product changes yourself") || strings.Contains(out, "Do not write product code yourself") {
		c.Contradicted(obsPolicyFrozen, "after the edit, guide answered %d, %v: %s", code, answered, out)
		return
	}
	if listing := workersOf(t, c, asks); !listing.MainImplements || listing.MainPolicySource != "session" {
		c.Contradicted(obsPolicyFrozen, "after the edit, workers: %+v", listing)
		return
	}
	c.Observed(obsPolicyFrozen, "a project edit left the live main's guide and workers policy as they were")

	// A fresh main takes the changed value; the first one is gone first, since
	// a room has one main.
	stopSession(t, c, lead)
	if !waitFor(c, 20*time.Second, func() bool { return noSessionRecords(iso) }) {
		c.Contradicted(obsPolicyFresh, "the first main's record outlived it")
		return
	}
	freshBrief := filepath.Join(iso.Home, "fresh.brief")
	freshAsks := newRequests(iso, "fresh")
	fresh := startSessionIn(t, c, iso, project, fixtureColumn.harness, "fresh", "--main", nil, nil,
		freshAsks.env(), shimInboxJSON+"=1", shimBriefFile+"="+freshBrief)
	defer stopSession(t, c, fresh)
	if !waitFor(c, 20*time.Second, func() bool { _, err := os.Stat(fresh.ready); return err == nil }) {
		c.Contradicted(obsPolicyFresh, "the fresh main never became ready")
		return
	}
	freshText := ""
	if !waitFor(c, 20*time.Second, func() bool {
		raw, err := os.ReadFile(freshBrief)
		freshText = string(raw)
		return err == nil && freshText != ""
	}) {
		c.Contradicted(obsPolicyFresh, "the fresh main received no briefing")
		return
	}
	if !strings.Contains(freshText, "the project's [main] implements = false") ||
		!strings.Contains(freshText, "Do not write product code yourself") {
		c.Contradicted(obsPolicyFresh, "the fresh briefing: %s", freshText)
		return
	}
	if listing := workersOf(t, c, freshAsks); listing.MainImplements || listing.MainPolicySource != "session" {
		c.Contradicted(obsPolicyFresh, "the fresh main's workers: %+v", listing)
		return
	}
	c.Observed(obsPolicyFresh, "a fresh main read the changed project policy and went back to delegating")
}

// writeMainPolicyProject writes the project's aliases: a main policy and one
// general reviewer, and nothing that could implement.
func writeMainPolicyProject(t *testing.T, project string, implements bool) {
	t.Helper()
	value := map[bool]string{true: "true", false: "false"}[implements]
	text := fmt.Sprintf("[main]\nimplements = %s\n[alias.reviewer]\nharness = %q\n", value, fixtureColumn.harness)
	path := filepath.Join(project, ".rewake", "aliases.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

// workersOf runs workers as the session and answers its machine form.
func workersOf(t *testing.T, c *Case, asks *requests) workersPolicy {
	t.Helper()
	code, out, answered := asks.ask(c, "workers", "--json")
	var listing workersPolicy
	if !answered || code != 0 || json.Unmarshal([]byte(out), &listing) != nil {
		c.Contradicted(obsPolicyGuideWorkers, "workers answered %d, %v: %s", code, answered, out)
	}
	return listing
}

type workersPolicy struct {
	NoWriter         bool   `json:"noWriter"`
	MainImplements   bool   `json:"mainImplements"`
	MainPolicySource string `json:"mainPolicySource"`
	Workers          []struct {
		Name string `json:"name"`
	} `json:"workers"`
}

// noSessionRecords says the room's session records are all gone: a main may
// start there again.
func noSessionRecords(iso *Isolation) bool {
	entries, err := os.ReadDir(filepath.Join(iso.Room(iso.HomeRoom()), "sessions"))
	if err != nil {
		return true
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			return false
		}
	}
	return true
}

const fixtureMainPolicyReview = "fixture-main-policy-probe: review the change and report"

const (
	obsPolicyStartup         = "main's briefing, launched under the project's opt-in, states the policy and the implementing rule"
	obsPolicyGuideWorkers    = "main's guide and workers answer with the same live policy, and noWriter stays inventory"
	obsPolicyReviewDelegated = "main delegates a review, the reviewer reads it, and its finished report reaches main"
	obsPolicyFrozen          = "editing the project file after the launch changes neither the live main's guide nor its listing"
	obsPolicyFresh           = "a fresh main in the same project takes the changed policy and delegates again"
)

var fixtureMainPolicyObservations = []string{obsPolicyStartup, obsPolicyGuideWorkers, obsPolicyReviewDelegated, obsPolicyFrozen, obsPolicyFresh}
