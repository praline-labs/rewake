package gateway

import (
	"context"
	"fmt"
	"testing"
)

// The terminal's configuration marks its selection only in the form its
// builder writes: an object holding web_search as one of the four modes. A
// configuration that is merely there, or holds anything else, marks nothing.
func TestTheTerminalsConfigurationMarksItsSelection(t *testing.T) {
	start := func(id, config string) string {
		return fmt.Sprintf(`{"id":%s,"method":"thread/start","params":{"threadSource":"user","runtimeWorkspaceRoots":null,"permissions":null,"config":%s}}`, id, config)
	}
	resume := func(id, params string) string {
		return fmt.Sprintf(`{"id":%s,"method":"thread/resume","params":{"threadId":"A","runtimeWorkspaceRoots":null,"permissions":null,%s}}`, id, params)
	}
	for _, c := range []struct {
		raw  string
		want bool
	}{
		{start("1", `{"web_search":"disabled"}`), true},
		{start("1", `{"web_search":"cached"}`), true},
		{start("1", `{"web_search":"indexed"}`), true},
		{start("1", `{"web_search":"live"}`), true},
		{start(`"startup-thread-start-1"`, `{"web_search":"cached"}`), true},
		// 0.155.1 sends a personality beside it, and a launch's own -c keys
		// come along too.
		{start("1", `{"personality":"pragmatic","model_reasoning_effort":"high","web_search":"cached"}`), true},
		{start("1", `{}`), false},
		{start("1", `null`), false},
		{start("1", `{"personality":"pragmatic"}`), false},
		{start("1", `{"web_search":null}`), false},
		{start("1", `{"web_search":1}`), false},
		{start("1", `{"web_search":["cached"]}`), false},
		{start("1", `{"web_search":"sometimes"}`), false},
		{start("1", `{"features":{"web_search":"cached"}}`), false},
		{start("1", `"web_search"`), false},
		// Helpers are built from the same configuration.
		{start(`"tui-dynamic-1"`, `{"web_search":"cached"}`), false},
		{start(`"temporary-1"`, `{"web_search":"cached"}`), false},
		{`{"id":1,"method":"thread/start","params":{"threadSource":"subagent","config":{"web_search":"cached"}}}`, false},
		{resume("5", `"config":{"web_search":"cached"},"history":null,"path":null,"excludeTurns":true`), true},
		// The serializer leaves excludeTurns out when it is false, and the
		// history and the path when they are none.
		{resume("5", `"config":{"web_search":"cached"},"history":null,"path":null`), true},
		{resume("5", `"config":{"web_search":"cached"}`), true},
		{resume("5", `"config":{"web_search":"cached"},"history":[],"path":null`), false},
		{resume("5", `"config":{"web_search":"cached"},"history":null,"path":"/rollout.jsonl"`), false},
		{resume("5", `"config":{},"history":null,"path":null`), false},
		{resume(`"tui-dynamic-2"`, `"config":{"web_search":"cached"},"history":null,"path":null`), false},
		{resume(`"temporary-2"`, `"config":{"web_search":"cached"},"history":null,"path":null`), false},
		{`{"id":5,"method":"thread/resume","params":{"threadId":"","config":{"web_search":"cached"}}}`, false},
		// A resume with defaults only is the terminal rejoining: a selection
		// only as a correlated reconnect.
		{`{"id":5,"method":"thread/resume","params":{"threadId":"A"}}`, false},
		{`{"id":5,"method":"thread/resume","params":{"threadId":"A","config":{}}}`, false},
		// A rejoin through the terminal's own tool server adds that server to
		// the configuration, and nothing else (0.157.1,
		// app_server_session.rs:248, rollout_history.rs:157).
		{`{"id":5,"method":"thread/resume","params":{"threadId":"A","config":{"mcp_servers.codex_tui":{"url":"http://127.0.0.1:1"}}}}`, false},
	} {
		if got := recognized(metadata(t, c.raw)); got != c.want {
			t.Errorf("recognized %v, want %v: %s", got, c.want, c.raw)
		}
	}
}

// A field the recognition reads is refused twice over, inside the
// configuration as well: a reader taking the first and one taking the last
// would disagree about what the request is.
func TestDuplicateSelectionFieldsAreRefused(t *testing.T) {
	for _, raw := range []string{
		`{"id":1,"method":"thread/start","params":{"threadSource":"user","config":{"web_search":"disabled","web_search":"cached"}}}`,
		`{"id":1,"method":"thread/resume","params":{"threadId":"A","config":{"web_search":"cached"},"history":null,"history":[]}}`,
		`{"id":1,"method":"thread/resume","params":{"threadId":"A","config":{"web_search":"cached"},"path":null,"path":"/rollout.jsonl"}}`,
		`{"id":1,"method":"thread/resume","params":{"threadId":"A","config":{"web_search":"cached"},"excludeTurns":true,"excludeTurns":false}}`,
	} {
		if _, err := project([]byte(raw)); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}

// A helper of the terminal's carries the terminal's configuration, and still
// does not take the selection.
func TestAHelperWithTheTerminalsConfigurationDoesNotSelect(t *testing.T) {
	s := newState("epoch", 1)
	req(t, &s, `{"id":1,"method":"thread/start","params":{"threadSource":"user","runtimeWorkspaceRoots":null,"config":{"web_search":"cached"}}}`)
	answer(t, &s, "1", "A")
	for i, id := range []string{`"tui-dynamic-1"`, `"temporary-1"`} {
		req(t, &s, fmt.Sprintf(`{"id":%s,"method":"thread/start","params":{"threadSource":"user","runtimeWorkspaceRoots":null,"config":{"web_search":"cached"}}}`, id))
		answer(t, &s, id, fmt.Sprint("helper-", i))
		if !s.Ready || s.Thread != "A" {
			t.Fatalf("after %s: %+v", id, s.Binding)
		}
	}
}

// The terminal's fork carries its configuration as its start does; read in
// the source of 0.157.1 — no fork was driven live.
func TestAForkWithTheTerminalsConfigurationIsAFork(t *testing.T) {
	if !forkIntent(metadata(t, `{"id":3,"method":"thread/fork","params":{"threadId":"A","threadSource":"user","runtimeWorkspaceRoots":null,"permissions":null,"config":{"web_search":"cached"}}}`)) {
		t.Fatal("the terminal's fork of 0.157.1 was not taken for one")
	}
	if forkIntent(metadata(t, `{"id":3,"method":"thread/fork","params":{"threadId":"A","threadSource":"user","runtimeWorkspaceRoots":null,"config":{}}}`)) {
		t.Fatal("a fork with an empty configuration was taken for the terminal's")
	}
}

// An ordinary resume of 0.157.1 on a new connection, of the thread the last
// one had selected, is the terminal's /resume, not a reconnect: it carries the
// terminal's configuration, and the reads that follow it are its backfill. As
// a reconnect it would allow no backfill, and the first read of another
// loaded conversation would undo the selection.
func TestAnOrdinaryResumeAfterACloseIsNotAReconnect(t *testing.T) {
	g, ui, peers, path := setup(t)
	native := <-peers
	bindUI(t, g, ui, native)
	_ = ui.conn.Close()
	_ = native.conn.Close()
	awaitReleased(t, g)
	next, err := dialSocket(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = next.conn.Close() }()
	peer := <-peers
	defer func() { _ = peer.conn.Close() }()
	exchange(t, next, peer, `{"id":0,"method":"initialize"}`, `{"id":0,"result":{}}`)
	exchange(t, next, peer, `{"id":1,"method":"thread/resume","params":{"threadId":"A","runtimeWorkspaceRoots":null,"permissions":null,"config":{"web_search":"cached"},"history":null,"path":null,"excludeTurns":true}}`,
		`{"id":1,"result":{"thread":{"id":"A","canAcceptDirectInput":true,"status":{"type":"idle"}}}}`)
	exchange(t, next, peer, `{"id":2,"method":"thread/loaded/list","params":{"cursor":null,"limit":null}}`, `{"id":2,"result":{"data":["A","B"],"nextCursor":null}}`)
	exchange(t, next, peer, `{"id":3,"method":"thread/read","params":{"threadId":"B"}}`, `{"id":3,"result":{"thread":{"id":"B","canAcceptDirectInput":true,"status":{"type":"idle"}}}}`)
	assertTarget(t, g, "A")
}
