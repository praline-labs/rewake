package host

import (
	"context"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/host/delivery"
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
	delivered := make(chan mail.Message, 4)
	ready, done := make(chan struct{}), make(chan struct{})
	server := &delivery.Server{Dir: dir, Name: main.Name, Epoch: main.Epoch(), Ready: func() { close(ready) }, Deliver: func(_ context.Context, m mail.Message) mail.Result {
		delivered <- m
		return mail.Result{State: mail.Delivered}
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
