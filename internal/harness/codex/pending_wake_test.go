package codex

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/role"
)

func TestNativeRejectsUnsupportedExplicitGrants(t *testing.T) {
	for _, part := range []role.Role{role.General, role.Write} {
		kinds := []inbox.Kind{inbox.Note, inbox.Finished, inbox.Error, inbox.Stopped}
		if part.ID == role.General.ID {
			kinds = append(kinds, inbox.Task, inbox.Question)
		}
		for _, kind := range kinds {
			t.Run(part.ID+"/"+string(kind), func(t *testing.T) {
				backend, captured := gitDeliveryFixture(t, part)
				result := backend.Deliver(context.Background(), inbox.Message{ID: "invalid-grant", Kind: kind, GrantGit: true})
				if result.State != inbox.Failed || !strings.Contains(result.Detail, "unsupported") {
					t.Fatal(result)
				}
				select {
				case <-captured:
					t.Fatal("invalid grant injected native work")
				default:
				}
			})
		}
	}
}

func TestExpiredExplicitMemberCannotGrantRightsToReportGroup(t *testing.T) {
	backend, captured := gitDeliveryFixture(t, role.Write)
	dir := t.TempDir()
	s := &inbox.Server{Dir: dir, Name: "receiver", Epoch: "epoch", Reserve: backend.Reserve}
	expired := inbox.Message{ID: inbox.NewID(), To: s.Name, ToEpoch: s.Epoch, Kind: inbox.Task, GrantGit: true, CreatedAt: time.Now().Add(-time.Hour)}
	report := inbox.Message{ID: inbox.NewID(), To: s.Name, ToEpoch: s.Epoch, Kind: inbox.Finished, CreatedAt: time.Now()}
	for _, m := range []inbox.Message{expired, report} {
		if err := inbox.Put(dir, m); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); s.Serve(ctx) }()
	defer func() { cancel(); <-done }()
	select {
	case params := <-captured:
		if _, ok := params["runtimeWorkspaceRoots"]; ok {
			t.Fatal("expired member expanded report permissions")
		}
		var id string
		_ = json.Unmarshal(params["clientUserMessageId"], &id)
		if id != report.ID {
			t.Fatal("expired member entered native group")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no report signal")
	}
}
