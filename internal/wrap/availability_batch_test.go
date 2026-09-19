package wrap

import (
	"context"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/role"
)

func TestLateMainDiscoveryPassGetsOneGroupedWake(t *testing.T) {
	dir := stateDir(t)
	for i, name := range []string{"first", "second", "third"} {
		availabilityPeer(t, dir, name, role.General.ID, uint64(i+1), true)
	}
	main := availabilityPeer(t, dir, "main-fixture", role.Main.ID, 10, true)
	main.StartedAt = time.Now()
	if err := registry.Update(dir, main); err != nil {
		t.Fatal(err)
	}
	delivered := make(chan inbox.Message, 4)
	ready, done := make(chan struct{}), make(chan struct{})
	server := &inbox.Server{Dir: dir, Name: main.Name, Epoch: main.Epoch(), Ready: func() { close(ready) }, Deliver: func(_ context.Context, m inbox.Message) inbox.Result {
		delivered <- m
		return inbox.Result{State: inbox.Delivered}
	}}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { defer close(done); server.Serve(ctx) }()
	defer func() { cancel(); <-done }()
	<-ready
	if err := announceAvailable(context.Background(), dir, main, map[string]registry.Session{}); err != nil {
		t.Fatal(err)
	}
	select {
	case group := <-delivered:
		if len(group.Batch) != 3 {
			t.Fatal("discovery pass split into separate wakes")
		}
		for _, m := range group.Batch {
			if m.Availability == nil || !m.Availability.Existing {
				t.Fatal("lost original availability identity")
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no discovery wake")
	}
}
