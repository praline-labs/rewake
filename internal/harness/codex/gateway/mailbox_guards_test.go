package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// A different error, such as admission expiry, must not satisfy either guard.
func TestOversizedMailboxNoticeRejectedBySizeGuard(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r, err := g.Reserve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Prepare(func(string) error { return nil }); err != nil {
		t.Fatalf("guard needs a live reservation: %v", err)
	}
	_, err = r.Deliver(ctx, "id", MailboxNotice{Notice: strings.Repeat("x", (64<<10)+1)}, nil)
	if err == nil || err.Error() != "invalid notice" {
		t.Fatalf("oversized notice refused by the wrong guard: %v", err)
	}
}

func TestMailboxReplayRejectedBySingleSendGuard(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r, err := g.Reserve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	notice := MailboxNotice{Notice: "ok", Members: []MailboxMember{{ID: "m"}}}
	done := make(chan error, 1)
	go func() { _, err := r.Deliver(ctx, "id", notice, nil); done <- err }()
	var request struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(readWithin(t, native), &request) != nil {
		t.Fatal("invalid request")
	}
	write(t, native, []byte(fmt.Sprintf(`{"id":%q,"result":{"turn":{"id":"work"}}}`, request.ID)))
	if err = <-done; err != nil {
		t.Fatalf("first delivery failed: %v", err)
	}
	if err := r.Prepare(func(string) error { return nil }); err != nil {
		t.Fatalf("guard needs a live reservation: %v", err)
	}
	_, err = r.Deliver(ctx, "id", notice, nil)
	if err == nil || err.Error() != "delivery reservation already used; do not replay" {
		t.Fatalf("replay refused by the wrong guard: %v", err)
	}
}
