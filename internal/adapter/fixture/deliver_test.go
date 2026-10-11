//go:build rewakefixture

package fixture

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// No notice precedes a reserved letter's readability: through the mailbox
// server the wrapper runs, the program is asked to reserve while the letter is
// not yet readable, and when the notice reaches it the letter is: what holds
// for any Reserver.
func TestNoNoticePrecedesAReservedLettersReadability(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(state.DirEnv, dir)
	dir, err := state.Dir()
	if err != nil {
		t.Fatal(err)
	}
	b, p := paired(t, adapter.TurnHandler{}, Served...)
	var reserving adapter.Reserver = b
	server := &mail.Server{Dir: dir, Name: "api", Epoch: "1.1", Thread: b.Thread, Deliver: b.Deliver, Reserve: reserving.Reserve}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan struct{})
	go func() { defer close(served); server.Serve(ctx) }()
	t.Cleanup(func() { cancel(); <-served })

	task := mail.Message{ID: mail.NewID(), From: "web", FromEpoch: "2.2", To: "api", ToEpoch: "1.1", Kind: mail.Task, Text: "work", CreatedAt: time.Now()}
	if err := mail.Put(dir, task); err != nil {
		t.Fatal(err)
	}
	readable := func() bool {
		unread, err := mail.PeekUnread(dir, "api", "1.1")
		return err == nil && slices.ContainsFunc(unread, func(m mail.Message) bool { return m.ID == task.ID })
	}
	reserve := p.read(t)
	if reserve.Op != opReserve {
		t.Fatalf("the first request was %s", reserve.Op)
	}
	if readable() {
		t.Fatal("the letter was readable before its reservation was answered")
	}
	p.write(t, Frame{Op: opAnswer, ID: reserve.ID, OK: true, Thread: programThrd})
	deliver := p.read(t)
	if deliver.Op != opDeliver || deliver.Thread != programThrd || len(deliver.Members) != 1 || deliver.Members[0].ID != task.ID {
		t.Fatalf("the notice: %+v", deliver)
	}
	if !readable() {
		t.Fatal("the notice preceded the letter's readability")
	}
	p.write(t, Frame{Op: opAnswer, ID: deliver.ID, OK: true})
	if release := p.read(t); release.Op != opRelease {
		t.Fatalf("the reservation was not let go: %+v", release)
	}
	eventually(t, "the delivery", func() bool {
		status, ok, _ := mail.ReadStatus(dir, "api", task.ID)
		return ok && status.State == mail.Delivered
	})
}
