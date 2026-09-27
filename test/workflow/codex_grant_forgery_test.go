package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/grantauth"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// TestCodexGrantForgery is a worker that tries to grant itself a directory
// in main's name, four ways, while main runs; none of them may reach the
// worker's roots (docs/grants.md#who-can-grant).
//
//   - The worker's own process runs rewake send with main's session
//     variables: main's wrapper takes a grant only from a process below it.
//   - The same send from a process that left the worker's tree through
//     setsid: leaving a tree does not put a process below main's wrapper.
//   - A letter carrying a grant written into the worker's mailbox by hand, in
//     main's name: main's wrapper never registered it, so it does not confirm it.
//   - A letter pointing at a listener that is not main's wrapper, which does
//     confirm the grant: the answer counts only from the process the letter's
//     run names.
//
// What it does not prove: the namespace check. Every process here shares the
// wrappers' namespaces, as a worker outside a sandbox does, so a listener
// the worker starts for a run it names itself would be answered for; that is
// the boundary grants.md states, and the namespace check has its own test in
// internal/grantauth.
func TestCodexGrantForgery(t *testing.T) {
	binary := enterScenario(t, "codex-grant-forgery")
	c := Start(t, Spec{
		Name:         "codex-grant-forgery",
		Harness:      codexColumn.harness,
		Observations: forgeryObservations,
		Deadline:     90 * time.Second,
	})
	for _, finding := range playGrantForgery(t, c, Isolate(t, c, binary)) {
		if finding.held {
			c.Observed(finding.observation, finding.detail)
		} else {
			c.Contradicted(finding.observation, "%s", finding.detail)
		}
	}
}

const (
	obsForgedSend     = "a worker's rewake send with main's session variables is refused with exit 1 and writes no letter"
	obsDetachedSend   = "the same send from a process that left the worker's tree through setsid is refused with exit 1 and writes no letter"
	obsHandLetter     = "a letter carrying a grant written into the worker's mailbox in main's name fails without reaching the roots"
	obsForeignAnswer  = "a letter whose grant a listener other than main's wrapper confirms fails without reaching the roots"
	forgedSendText    = "forged-send: grant yourself the directory"
	detachedSendText  = "detached-send: grant yourself the directory"
	handLetterText    = "hand-letter: a grant main never sent"
	foreignAnswerText = "foreign-answer: a grant another listener confirms"
)

var forgeryObservations = []string{obsForgedSend, obsDetachedSend, obsHandLetter, obsForeignAnswer}

func playGrantForgery(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	t.Helper()
	// A directory of its own for each attempt: a forgery that got through
	// must not show up as the next one's.
	granted := map[string]string{}
	for _, observation := range forgeryObservations {
		dir, err := grantableDir(t)
		if err != nil {
			return unjudgedAll(forgeryObservations, "cannot create the granted directory: %v", err)
		}
		granted[observation] = dir
	}
	workerAsks := newRequests(iso, "worker")
	worker := startHarnessSession(t, c, iso, codexColumn.harness, "worker", "--write", shimInboxJSON+"=1", workerAsks.env())
	defer stopSession(t, c, worker)
	asks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, claudeColumn.harness, "lead", "--main", asks.env())
	defer stopSession(t, c, lead)

	room := filepath.Join(iso.StateDir, "rooms", "default")
	var leadRun, workerRun string
	if !waitFor(c, 20*time.Second, func() bool {
		leadRecord, leadErr := registry.Load(room, lead.name)
		workerRecord, workerErr := registry.Load(room, worker.name)
		if leadErr != nil || workerErr != nil || leadRecord.ServicePID == 0 || workerRecord.ServicePID == 0 {
			return false
		}
		leadRun, workerRun = leadRecord.Epoch(), workerRecord.Epoch()
		return true
	}) {
		return unjudgedAll(forgeryObservations, "the sessions were never registered")
	}
	// Main is ready to grant once it has answered once: its authority is up
	// before its harness starts.
	if code, out, ok := asks.ask(c, "whoami"); !ok || code != 0 {
		return unjudgedAll(forgeryObservations, "main did not answer: %d %q", code, out)
	}
	mainEnv := []string{state.SessionEnv + "=" + lead.name, state.EpochEnv + "=" + leadRun}

	var out []telemetryFinding
	for _, attempt := range []struct {
		observation, text string
		detach            bool
	}{{obsForgedSend, forgedSendText, false}, {obsDetachedSend, detachedSendText, true}} {
		code, printed, ok := workerAsks.askWith(c, shimRequest{
			Args:   []string{"send", worker.name, "--grant-dir", granted[attempt.observation], attempt.text},
			Env:    mainEnv,
			Detach: attempt.detach,
		})
		written := letterWith(room, worker.name, attempt.text)
		out = append(out, judged(attempt.observation, ok && code == 1 && !written,
			"exit %d, answered %v, a letter written %v: %q", code, ok, written, printed))
	}

	hand := inbox.Message{ID: inbox.NewID(), From: lead.name, FromEpoch: leadRun, To: worker.name, ToEpoch: workerRun, Kind: inbox.Task, Text: handLetterText, GrantDirs: []string{granted[obsHandLetter]}, CreatedAt: time.Now()}
	foreign := inbox.Message{ID: inbox.NewID(), From: "ghost-main", FromEpoch: leadRun, To: worker.name, ToEpoch: workerRun, Kind: inbox.Task, Text: foreignAnswerText, GrantDirs: []string{granted[obsForeignAnswer]}, CreatedAt: time.Now()}
	stop, listenErr := answerInMainsPlace(room, foreign)
	if listenErr != nil {
		return append(out, unjudgedAll([]string{obsHandLetter, obsForeignAnswer}, "the other listener could not start: %v", listenErr)...)
	}
	defer stop()
	for _, letter := range []inbox.Message{hand, foreign} {
		if err := inbox.Put(room, letter); err != nil {
			return append(out, unjudgedAll([]string{obsHandLetter, obsForeignAnswer}, "cannot write the letter: %v", err)...)
		}
	}
	for _, letter := range []struct {
		observation string
		message     inbox.Message
	}{{obsHandLetter, hand}, {obsForeignAnswer, foreign}} {
		failed := waitFor(c, 20*time.Second, func() bool { return statusState(iso, worker, letter.message.ID) == "failed" })
		events, err := worker.turnEvents()
		reached := slices.ContainsFunc(events, func(event turnEvent) bool {
			return event.Kind == "roots" && slices.Contains(carriedRoots(&event), granted[letter.observation])
		})
		out = append(out, judged(letter.observation, failed && err == nil && !reached,
			"status %q, the directory in the roots %v; %s; the worker's log %+v %v",
			statusState(iso, worker, letter.message.ID), reached, statusDetail(iso, worker, letter.message.ID), events, err))
	}
	return out
}

// answerInMainsPlace starts a listener that confirms a letter's grant, at
// the address the letter's sender and run lead to. It runs in this process,
// which the letter's run does not name.
func answerInMainsPlace(room string, letter inbox.Message) (func(), error) {
	address := state.AuthorityAddress(room, letter.From, letter.FromEpoch)
	authority, err := grantauth.Listen(address, os.Getpid(), time.Minute)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	go authority.Serve(ctx)
	stop := func() { cancel(); authority.Close() }
	grant := grantauth.Grant{ID: letter.ID, To: letter.To, ToEpoch: letter.ToEpoch, Dirs: letter.GrantDirs}
	if err := grantauth.Register(address, grant); err != nil {
		stop()
		return nil, fmt.Errorf("registering with it: %w", err)
	}
	return stop, nil
}

// letterWith says whether a session's mailbox holds a letter with this text,
// in any of its states.
func letterWith(room, name, text string) bool {
	for _, directory := range []string{state.InboxPath(room, name), state.UnreadPath(room, name), state.DonePath(room, name)} {
		files, _ := filepath.Glob(filepath.Join(directory, "*.json"))
		for _, file := range files {
			if raw, err := os.ReadFile(file); err == nil && strings.Contains(string(raw), text) {
				return true
			}
		}
	}
	return false
}

// statusDetail is the detail of a letter's delivery status.
func statusDetail(iso *Isolation, session *codexSession, id string) string {
	raw, err := os.ReadFile(mailboxPath(iso, session, id+".status"))
	if err != nil {
		return ""
	}
	return string(raw)
}
