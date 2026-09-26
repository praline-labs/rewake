package gateway

import (
	"fmt"
	"testing"
	"time"
)

func metadata(t *testing.T, raw string) meta {
	t.Helper()
	m, e := project([]byte(raw))
	if e != nil {
		t.Fatal(e)
	}
	return m
}

func req(t *testing.T, s *state, raw string) {
	t.Helper()
	if e := s.request(metadata(t, raw)); e != nil {
		t.Fatal(e)
	}
}

func answer(t *testing.T, s *state, id, thread string) {
	t.Helper()
	s.response(metadata(t, fmt.Sprintf(`{"id":%s,"result":{"thread":{"id":%q,"canAcceptDirectInput":true,"status":{"type":"idle"}}}}`, id, thread)))
}

// legacy(codex <0.157.1): the terminal's start as 0.155.1 sends it, which most tests here take for any terminal's; remove when 0.155.1 is no longer supported, rewriting those tests in the 0.157.1 form
const startA = `{"id":1,"method":"thread/start","params":{"threadSource":"user","runtimeWorkspaceRoots":[],"config":{},"input":"SECRET"}}`

func TestAcceptedIntentABAStaleRepliesAndRefusals(t *testing.T) {
	s := newState("epoch", 1)
	req(t, &s, startA)
	answer(t, &s, "1", "A")
	for i, thread := range []string{"B", "A"} {
		id := i + 2
		req(t, &s, fmt.Sprintf(`{"id":%d,"method":"thread/resume","params":{"threadId":%q,"config":{},"runtimeWorkspaceRoots":[]}}`, id, thread))
		if s.Ready {
			t.Fatal("pending intent remained ready")
		}
		answer(t, &s, fmt.Sprint(id), thread)
		if !s.Ready || s.Thread != thread {
			t.Fatal(s.Binding)
		}
	}
	req(t, &s, `{"id":"startup-thread-start-9","method":"thread/start","params":{"threadSource":"user","runtimeWorkspaceRoots":[]}}`)
	req(t, &s, `{"id":9,"method":"thread/resume","params":{"threadId":"B","runtimeWorkspaceRoots":[],"config":{}}}`)
	answer(t, &s, `"startup-thread-start-9"`, "old")
	if s.Ready {
		t.Fatal("late startup rebound")
	}
	s.response(metadata(t, `{"id":9,"error":{"code":1,"message":"SECRET"}}`))
	if s.Ready {
		t.Fatal("error restored old target")
	}
	req(t, &s, startA)
	s.response(metadata(t, `{"id":1,"result":{"thread":{"id":"A","canAcceptDirectInput":false}}}`))
	if s.Ready {
		t.Fatal("read-only target")
	}
	req(t, &s, startA)
	s.response(metadata(t, `{"id":1,"result":{"thread":{"id":"A"}}}`))
	if s.Ready {
		t.Fatal("unknown capability")
	}
}

func TestUnknownAndBackgroundDoNotSelect(t *testing.T) {
	s := newState("epoch", 1)
	req(t, &s, startA)
	answer(t, &s, "1", "A")
	req(t, &s, `{"id":"tui-dynamic-1","method":"thread/start","params":{"threadSource":"user","runtimeWorkspaceRoots":[]}}`)
	answer(t, &s, `"tui-dynamic-1"`, "child")
	if s.Thread != "A" {
		t.Fatal(s.Binding)
	}
	for _, raw := range []string{`{"id":5,"method":"thread/resume","params":{"threadId":"A","config":{}}}`, `{"id":6,"method":"thread/read","params":{"threadId":"B","includeTurns":true}}`, `{"id":7,"method":"thread/fork","params":{"threadId":"A"}}`, `{"id":8,"method":"thread/futureControl","params":{}}`} {
		req(t, &s, raw)
		if s.Ready {
			t.Fatal("ambiguous operation retained readiness")
		}
	}
	if e := s.request(metadata(t, `{"id":8,"method":"thread/list"}`)); e == nil {
		t.Fatal("duplicate id accepted")
	}
}

func TestObservedActiveBeforeBindingKeepsGapAndFiltersOldThreads(t *testing.T) {
	now := time.Now()
	o := newObserver()
	o.event(meta{method: "thread/status/changed", thread: "A", status: "active"}, nil, now)
	o.event(meta{method: "thread/status/changed", thread: "A", status: "idle"}, nil, now)
	o.bind("A", "idle", now)
	o.expire(now.Add(time.Second))
	out := o.drain()
	if len(out) != 1 || out[0].Kind != "stopped" || out[0].Text != gapText {
		t.Fatal(out)
	}
	o.expire(now.Add(2 * time.Second))
	if len(o.drain()) != 0 {
		t.Fatal("duplicate gap")
	}
	o.event(meta{method: "turn/completed", thread: "A", turn: "late", status: "completed"}, nil, now)
	if len(o.drain()) != 0 {
		t.Fatal("late duplicate")
	}
	o.event(meta{method: "turn/started", thread: "child", turn: "1"}, nil, now)
	o.event(meta{method: "turn/completed", thread: "child", turn: "1", status: "completed"}, nil, now)
	if len(o.drain()) != 0 {
		t.Fatal("nested report")
	}
}

func TestLiveCompletionOnlyAndCompaction(t *testing.T) {
	now := time.Now()
	for _, status := range []string{"completed", "failed", "interrupted"} {
		o := newObserver()
		o.bind("A", "idle", now)
		o.event(meta{method: "turn/started", thread: "A", turn: "1"}, nil, now)
		o.event(meta{method: "item/completed", thread: "A", turn: "1"}, []byte(`{"params":{"item":{"type":"agentMessage","text":"live final"}}}`), now)
		o.event(meta{method: "thread/status/changed", thread: "A", status: "idle"}, nil, now)
		o.event(meta{method: "turn/completed", thread: "A", turn: "1", status: status}, nil, now)
		out := o.drain()
		if len(out) != 1 {
			t.Fatal(out)
		}
		if status == "completed" && out[0].Text != "live final" {
			t.Fatal(out)
		}
		o.expire(now.Add(time.Second))
		if len(o.drain()) != 0 {
			t.Fatal("duplicate gap after outcome")
		}
	}
	o := newObserver()
	o.bind("A", "idle", now)
	o.event(meta{method: "turn/started", thread: "A", turn: "compact"}, nil, now)
	o.event(meta{method: "turn/completed", thread: "A", turn: "compact", status: "completed"}, nil, now)
	o.expire(now.Add(time.Second))
	c := &connection{admitted: newAdmittedWork()}
	if len(c.proven(o.drain(), time.Now())) != 0 {
		t.Fatal("compaction reported task completion")
	}
}
