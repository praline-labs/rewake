package delivery

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
)

// A grant whose message outlived its wait is refused before main is asked,
// and its sender is told: a grant main let go after its lifetime would be
// refused anyway, and asking a busy main only delays the answer.
func TestAnExpiredGrantIsRefusedWithoutAskingMain(t *testing.T) {
	dir := stateDir(t)
	task := grantPending(t, dir, true)[0]
	s := batchServer(dir)
	s.TTL = time.Second
	asked := false
	s.CheckGrant = func(mail.Message) error { asked = true; return nil }
	s.Deliver = func(context.Context, mail.Message) mail.Result {
		t.Fatal("an expired grant was delivered")
		return mail.Result{}
	}
	s.drain(context.Background())
	if asked {
		t.Error("main was asked about an expired grant")
	}
	status, ok, _ := mail.ReadStatus(dir, "api", task.ID)
	if !ok || status.State != mail.Failed || !strings.Contains(status.Detail, "expired") {
		t.Fatalf("status = %+v", status)
	}
	told, err := mail.List(dir, "web")
	if err != nil || len(told) != 1 || told[0].Undelivered == nil || told[0].Undelivered.ID != task.ID {
		t.Fatalf("sender was not told: %+v %v", told, err)
	}
}
