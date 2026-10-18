package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/authority"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/build"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// Each form of decide is the access class it is (docs/v2/stage3-decision.md
// #the-command): the list, its JSON, the help and every wrong call are views
// that change neither a missing base nor a populated one, wait for no lease
// and say once that the room is another build's; a choice is a change and
// needs the lease. Through the tool a choice runs under a receipt bound to the
// call: a wrapper out of reach leaves it unfinished, nothing decided and the
// person never asked, and retry with its token decides once, verified.
func TestDecisionFormAccessAndToolExclusion(t *testing.T) {
	views := func(report string) [][]string {
		return [][]string{
			{"decide", "api"},
			{"decide", "api", "--json"},
			{"decide", "--help"},
			{"decide"},
			{"decide", "api", report},
			{"decide", "api", report, "--arrived", "--not-arrived"},
		}
	}

	t.Run("a missing base", func(t *testing.T) {
		parent := t.TempDir()
		t.Setenv(state.DirEnv, filepath.Join(parent, "state"))
		t.Setenv(state.RoomEnv, "")
		t.Setenv(state.SessionEnv, "")
		t.Setenv(epochEnv, "")
		(&route{}).install(t)
		before := snapshotTree(t, parent)
		for _, argv := range views("1790370984617481194-8d4ddd85c8f1") {
			run(argv...)
			if after := snapshotTree(t, parent); after != before {
				t.Fatalf("%q changed a missing base:\n--- before\n%s\n--- after\n%s", argv, before, after)
			}
		}
	})

	t.Run("a populated base, a launch switching its build", func(t *testing.T) {
		lab := newDecideLab(t)
		r := &route{}
		r.install(t)
		buildtest.Release(lab.dir)
		holder := holdExclusive(t, lab.dir)
		defer holder()
		before := snapshotTree(t, lab.dir)
		for _, argv := range views(lab.report.ID) {
			if _, _, errOut := run(argv...); strings.Contains(errOut, "has not finished within") {
				t.Errorf("%q waited for the lease: %s", argv, errOut)
			}
			if after := snapshotTree(t, lab.dir); after != before {
				t.Fatalf("%q changed the room:\n--- before\n%s\n--- after\n%s", argv, before, after)
			}
		}
		if r.scanned != 0 || r.asked != 0 {
			t.Fatalf("a view asked who may decide: classified %d, the wrapper asked %d", r.scanned, r.asked)
		}
	})

	t.Run("another build's room", func(t *testing.T) {
		lab := newDecideLab(t)
		(&route{}).install(t)
		foreign := strings.Repeat("cd", 32)
		if err := os.WriteFile(filepath.Join(lab.dir, "build"), []byte(foreign+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		warning := "Rewake: room default belongs to build " + build.Short(foreign)
		for _, argv := range views(lab.report.ID) {
			if _, out, errOut := run(argv...); strings.Count(errOut, warning) != 1 || strings.Contains(out, warning) {
				t.Errorf("%q: %q on stderr, %q on stdout", argv, errOut, out)
			}
		}
		code, _, errOut := run("decide", "api", lab.report.ID, "--arrived")
		if code != ExitFailed || !strings.Contains(errOut, "belongs to build "+build.Short(foreign)) || lab.decision(t) != nil {
			t.Fatalf("a choice in another build's room: exit %d, %q", code, errOut)
		}
	})

	t.Run("through the tool, the wrapper out of reach, then retried", func(t *testing.T) {
		lab := newDecideLab(t)
		t.Setenv(state.SessionEnv, "web")
		reachable := false
		r := &route{below: true, wrapper: func(run authority.Run, ask mail.DecideRequest) (mail.DecideAnswer, error) {
			if !reachable {
				return mail.DecideAnswer{}, errors.Join(authority.ErrUnreachable, os.ErrPermission)
			}
			return lab.asWrapper(run, ask)
		}}
		r.install(t)
		caller := newToolCaller(t)
		first := caller.run("decide", "api", lab.report.ID, "--arrived")
		found := retryLine.FindStringSubmatch(first.errOut + first.out)
		if first.code == ExitOK || found == nil || lab.decision(t) != nil || r.scanned != 0 || len(caller.tickets) != 1 {
			t.Fatalf("unreachable: exit %d, classified %d, tickets %d:\n%s%s", first.code, r.scanned, len(caller.tickets), first.out, first.errOut)
		}
		reachable = true
		again := caller.run("retry", found[1])
		d := lab.decision(t)
		if again.code != ExitOK || d == nil || !d.By.Verified || d.Choice != mail.ChoiceArrived || r.asked != 2 {
			t.Fatalf("retried: exit %d, decision %+v, asked %d:\n%s%s", again.code, d, r.asked, again.out, again.errOut)
		}
		third := caller.run("retry", found[1])
		if third.code != ExitOK || r.asked != 2 || lab.decision(t).ID != d.ID {
			t.Fatalf("retried again: exit %d, asked %d:\n%s%s", third.code, r.asked, third.out, third.errOut)
		}
		if r.scanned != 0 {
			t.Fatalf("below the wrapper the person was classified %d times", r.scanned)
		}
	})
}
