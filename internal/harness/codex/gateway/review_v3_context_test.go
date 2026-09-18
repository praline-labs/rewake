package gateway

import (
	"context"
	"fmt"
	"testing"
)

func TestReviewV3PartialBackfillCannotOutliveOrdinaryUserTurn(t *testing.T) {
	s := newState("review", 1)
	resume(t, &s, 1, "A")
	listing(t, &s, 2, `{"data":["A","B","C"],"nextCursor":null}`)
	req(t, &s, `{"id":3,"method":"thread/read","params":{"threadId":"B"}}`)
	// The UI has left the borrowed backfill operation and accepts ordinary work.
	req(t, &s, `{"id":4,"method":"turn/start","params":{"threadId":"A"}}`)
	s.response(metadata(t, `{"id":4,"result":{"turn":{"id":"user-turn"}}}`))
	req(t, &s, `{"id":5,"method":"thread/read","params":{"threadId":"C"}}`)
	if s.Ready {
		t.Fatalf("unused cohort entry survived ordinary user work and exempted a later read: %+v", s.Binding)
	}
}

func TestReviewV3UnopenedBackfillCannotOutliveOrdinaryUserTurn(t *testing.T) {
	s := newState("review", 1)
	resume(t, &s, 1, "A")
	req(t, &s, `{"id":2,"method":"turn/start","params":{"threadId":"A"}}`)
	s.response(metadata(t, `{"id":2,"result":{"turn":{"id":"user-turn"}}}`))
	listing(t, &s, 3, `{"data":["A","B"],"nextCursor":null}`)
	req(t, &s, `{"id":4,"method":"thread/read","params":{"threadId":"B"}}`)
	if s.Ready {
		t.Fatalf("first-list allowance stayed armed beyond ordinary user work: %+v", s.Binding)
	}
}

func TestReviewV3InvalidListAndGenerationGuards(t *testing.T) {
	for _, body := range []string{`{"data":["A","B"],"nextCursor":""}`, `{"data":["A",null]}`, `{"data":["A","B"],"nextCursor":5}`, `{"data":["A","B","B"]}`} {
		s := newState("review", 1)
		resume(t, &s, 1, "A")
		listing(t, &s, 2, body)
		req(t, &s, `{"id":3,"method":"thread/read","params":{"threadId":"B"}}`)
		if s.Ready {
			t.Fatalf("invalid list accepted: %s", body)
		}
	}
	s := newState("review", 1)
	resume(t, &s, 1, "A")
	req(t, &s, `{"id":2,"method":"thread/loaded/list","params":{}}`)
	resume(t, &s, 3, "B")
	raw := `{"id":2,"result":{"data":["A","B","C"],"nextCursor":null}}`
	s.response(metadata(t, raw), []byte(raw))
	req(t, &s, `{"id":4,"method":"thread/read","params":{"threadId":"C"}}`)
	if s.Ready {
		t.Fatal("old generation list authorized a read")
	}
}

func TestReviewV3StaleCohortStillAdmitsDeliveryOnOldTarget(t *testing.T) {
	g, ui, peers, _ := setup(t)
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	bindUI(t, g, ui, server)
	exchange := func(request, response string) {
		write(t, ui, []byte(request))
		_ = readWithin(t, server)
		write(t, server, []byte(response))
		_ = readWithin(t, ui)
	}
	exchange(`{"id":2,"method":"thread/resume","params":{"threadId":"A","config":{},"runtimeWorkspaceRoots":[]}}`, `{"id":2,"result":{"thread":{"id":"A","canAcceptDirectInput":true}}}`)
	ticket := g.Binding()
	exchange(`{"id":3,"method":"thread/loaded/list","params":{}}`, `{"id":3,"result":{"data":["A","B","C"],"nextCursor":null}}`)
	exchange(`{"id":4,"method":"thread/read","params":{"threadId":"B"}}`, `{"id":4,"result":{"thread":{"id":"B"}}}`)
	exchange(`{"id":5,"method":"turn/start","params":{"threadId":"A","input":[{"type":"text","text":"fixture"}]}}`, `{"id":5,"result":{"turn":{"id":"user-work"}}}`)
	exchange(`{"id":6,"method":"thread/read","params":{"threadId":"C"}}`, `{"id":6,"result":{"thread":{"id":"C"}}}`)
	if !g.Binding().Ready {
		return
	}
	done := make(chan error, 1)
	go func() { _, err := g.Deliver(context.Background(), ticket, "later-task", "fixture"); done <- err }()
	injected := metadata(t, string(readWithin(t, server)))
	write(t, server, []byte(fmt.Sprintf(`{"id":%q,"result":{"turn":{"id":"accepted"}}}`, injected.idText)))
	err := <-done
	if err == nil {
		t.Fatalf("later C read consumed stale context; delivery still accepted to %s with generation %d", injected.thread, ticket.Generation)
	}
}
