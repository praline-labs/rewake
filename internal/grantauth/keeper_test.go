package grantauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/grant"
)

// keep serves a keeper for a wrapper at self whose hook answers what decide
// says, with settled standing in for the mailbox.
func keep(t *testing.T, self int, settled map[string]bool, decide func(json.RawMessage, []grant.Entry) Decision) (*Keeper, string, *[][]grant.Entry) {
	t.Helper()
	path := "@rewake-test/keep/" + t.Name() + "/" + strconv.Itoa(os.Getpid())
	keeper, err := Keep(path, self)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	mirrored := &[][]grant.Entry{}
	keeper.Decide = decide
	keeper.Settled = func(id string) bool { mu.Lock(); defer mu.Unlock(); return settled[id] }
	keeper.Mirror = func(entries []grant.Entry) { *mirrored = append(*mirrored, entries) }
	ctx, cancel := context.WithCancel(context.Background())
	go keeper.Serve(ctx)
	t.Cleanup(func() { cancel(); keeper.Close() })
	return keeper, path, mirrored
}

func outcomes(entries []grant.Entry) string {
	var parts []string
	for _, entry := range entries {
		parts = append(parts, entry.Message+":"+entry.Path+"="+entry.Outcome)
	}
	return strings.Join(parts, " ")
}

func TestAKeeperAnswersItsHookFromWhatItHolds(t *testing.T) {
	settled := map[string]bool{}
	var seen []grant.Entry
	decide := func(call json.RawMessage, entries []grant.Entry) Decision {
		seen = entries
		switch string(call) {
		case `"add"`:
			return Decision{Output: []byte(`{"added":true}`), Added: []string{"/g"}}
		case `"remove"`:
			return Decision{Output: []byte(`{"removed":true}`), Removed: []string{"/g"}}
		}
		return Decision{}
	}
	keeper, path, mirrored := keep(t, os.Getpid(), settled, decide)
	if err := keeper.Grant("m1", []string{"/g", "/h"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	// A retried delivery grants nothing twice.
	if err := keeper.Grant("m1", []string{"/g", "/h"}, time.Now()); err != nil || len(keeper.Entries()) != 2 {
		t.Fatalf("again: %v, %s", err, outcomes(keeper.Entries()))
	}
	output, err := Ask(path, ownExpect(t), json.RawMessage(`"add"`))
	if err != nil || string(output) != `{"added":true}` || len(seen) != 2 {
		t.Fatalf("asked: %s %v, the decision saw %s", output, err, outcomes(seen))
	}
	// Settled: /g, which the hook added, waits for the hook to take it out;
	// /h, never written in, ends at once.
	settled["m1"] = true
	if _, err := Ask(path, ownExpect(t), json.RawMessage(`"nothing"`)); err != nil {
		t.Fatal(err)
	}
	if got := outcomes(keeper.Entries()); got != "m1:/g=revoking m1:/h=revoked" {
		t.Fatalf("after the report: %s", got)
	}
	if _, err := Ask(path, ownExpect(t), json.RawMessage(`"remove"`)); err != nil {
		t.Fatal(err)
	}
	if got := outcomes(keeper.Entries()); got != "m1:/g=revoked m1:/h=revoked" {
		t.Fatalf("after the removal: %s", got)
	}
	if last := (*mirrored)[len(*mirrored)-1]; outcomes(last) != "m1:/g=revoked m1:/h=revoked" {
		t.Fatalf("the copy says %s", outcomes(last))
	}
}

// A hook of another session — a process that does not run below this
// wrapper — is told nothing.
func TestAKeeperAnswersOnlyBelowItsWrapper(t *testing.T) {
	keeper, path, _ := keep(t, stranger(t), nil, func(json.RawMessage, []grant.Entry) Decision {
		return Decision{Output: []byte(`{}`)}
	})
	if err := keeper.Grant("m1", []string{"/g"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	_, err := Ask(path, ownExpect(t), json.RawMessage(`"add"`))
	if !errors.Is(err, ErrNotConfirmed) || !strings.Contains(err.Error(), "only this session's harness") {
		t.Fatalf("answered a stranger: %v", err)
	}
}

// The hook believes only its own wrapper: a listener at the address that is
// not the process its run names is not asked.
func TestAHookAsksOnlyItsWrapper(t *testing.T) {
	_, path, _ := keep(t, os.Getpid(), nil, func(json.RawMessage, []grant.Entry) Decision {
		return Decision{Output: []byte(`{"allow":true}`)}
	})
	own := ownExpect(t)
	for name, expect := range map[string]Expect{
		"another pid":        {PID: stranger(t), Start: own.Start},
		"another start time": {PID: own.PID, Start: own.Start + 1},
	} {
		if output, err := Ask(path, expect, json.RawMessage(`"add"`)); !errors.Is(err, ErrNotConfirmed) || output != nil {
			t.Errorf("%s: %s %v", name, output, err)
		}
	}
}

// Past the most it keeps, a keeper refuses the next grant and keeps what it
// has: a grant forgotten could never be taken back.
func TestAFullKeeperRefusesTheNextGrant(t *testing.T) {
	keeper, _, _ := keep(t, os.Getpid(), nil, nil)
	for index := range grant.MaxLive {
		if err := keeper.Grant(fmt.Sprint("m", index), []string{fmt.Sprint("/d", index)}, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if err := keeper.Grant("extra", []string{"/extra"}, time.Now()); err == nil || !strings.Contains(err.Error(), "rewake keeps") {
		t.Fatalf("past the limit: %v", err)
	}
	if entries := keeper.Entries(); len(entries) != grant.MaxLive || !slices.ContainsFunc(entries, func(entry grant.Entry) bool { return entry.Message == "m0" }) {
		t.Fatalf("kept %d, the first among them %v", len(entries), entries[0])
	}
}

// A grant restored after a resume keeps where it came from, and one the
// session was started with is taken out through the hook, as one it added.
func TestARestoredGrantIsTakenBackThroughTheHook(t *testing.T) {
	settled := map[string]bool{}
	keeper, path, _ := keep(t, os.Getpid(), settled, func(json.RawMessage, []grant.Entry) Decision { return Decision{} })
	origin := grant.Entry{Message: "m1", At: time.Now(), Thread: "c1", From: "lead", FromEpoch: "1.1"}
	if err := keeper.GrantFrom(origin, []string{"/g"}, true); err != nil {
		t.Fatal(err)
	}
	if err := keeper.GrantFrom(grant.Entry{Message: "m2", At: time.Now()}, []string{"/h"}, false); err != nil {
		t.Fatal(err)
	}
	if entry := keeper.Entries()[0]; entry.Thread != "c1" || entry.From != "lead" || entry.FromEpoch != "1.1" {
		t.Fatalf("the origin was lost: %+v", entry)
	}
	settled["m1"], settled["m2"] = true, true
	if _, err := Ask(path, ownExpect(t), json.RawMessage(`"nothing"`)); err != nil {
		t.Fatal(err)
	}
	if got := outcomes(keeper.Entries()); got != "m1:/g=revoking m2:/h=revoked" {
		t.Fatalf("after the report: %s", got)
	}
}
