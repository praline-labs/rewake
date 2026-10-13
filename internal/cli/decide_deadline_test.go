package cli

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/boottime"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// A tool call's decision is bound by the call's deadline at the writer, after
// both lock waits, not only before the wrapper is asked: the sender's mailbox
// held past the deadline, nothing is decided and the operation stays open;
// rewake retry, a call with a bound of its own, decides.
func TestAToolDecisionStopsAtItsDeadline(t *testing.T) {
	lab := newDecideLab(t)
	t.Setenv(state.SessionEnv, "web")
	t.Setenv(state.EpochEnv, lab.web.Epoch())
	r := &route{below: true, wrapper: lab.asWrapper}
	r.install(t)
	call, ctx, _, errOut := expiredCall(t, "decide", "api", lab.report.ID, "--arrived")
	now := boottime.Now()
	ctx.scope.ticket.CalledBoot, ctx.scope.ticket.DeadlineBoot = now, now+int64(100*time.Millisecond)
	locked, unlocked := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(unlocked)
		_ = state.WithMailboxLock(context.Background(), lab.dir, "api", func() error {
			close(locked)
			time.Sleep(350 * time.Millisecond)
			return nil
		})
	}()
	<-locked
	err := handleDecide(ctx, call)
	<-unlocked
	if ctx.scope.remaining() > 0 {
		t.Fatal("setup: the deadline is still ahead")
	}
	if r.asked != 1 {
		t.Fatalf("setup: the wrapper was asked %d times", r.asked)
	}
	if d := lab.decision(t); d != nil {
		t.Fatalf("decision %s written past the call's deadline (%v)", d.ID, err)
	}
	var exit *ExitCodeError
	found := retryLine.FindStringSubmatch(errOut.String())
	if !errors.As(err, &exit) || exit.Code != ExitFailed || found == nil || !strings.Contains(errOut.String(), "deadline came before the decision was written") {
		t.Fatalf("the call answered %v:\n%s", err, errOut)
	}
	again := newToolCaller(t).run("retry", found[1])
	d := lab.decision(t)
	if again.code != ExitOK || d == nil || !d.By.Verified || r.asked != 2 {
		t.Fatalf("retried: exit %d, decision %+v, asked %d:\n%s%s", again.code, d, r.asked, again.out, again.errOut)
	}
}
