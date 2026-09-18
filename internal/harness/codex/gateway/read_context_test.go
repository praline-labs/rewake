package gateway

import (
	"fmt"
	"testing"
)

func resume(t *testing.T, s *state, id int, target string) {
	t.Helper()
	req(t, s, fmt.Sprintf(`{"id":%d,"method":"thread/resume","params":{"threadId":%q,"runtimeWorkspaceRoots":[],"config":{}}}`, id, target))
	answer(t, s, fmt.Sprint(id), target)
}

func listing(t *testing.T, s *state, id int, body string) {
	t.Helper()
	req(t, s, fmt.Sprintf(`{"id":%d,"method":"thread/loaded/list","params":{}}`, id))
	raw := fmt.Sprintf(`{"id":%d,"result":%s}`, id, body)
	s.response(metadata(t, raw), []byte(raw))
}

func TestObservedStartupAndNumericResumeBackfillOrdering(t *testing.T) {
	for _, flag := range []string{"", `,"includeTurns":false`} {
		s := newState("epoch", 1)
		req(t, &s, `{"id":"startup-thread-start-fixture","method":"thread/start","params":{"threadSource":"user","runtimeWorkspaceRoots":[]}}`)
		req(t, &s, fmt.Sprintf(`{"id":"12345678-abcd-4321-aaaa-123456789abc","method":"thread/read","params":{"threadId":"A"%s}}`, flag))
		answer(t, &s, `"startup-thread-start-fixture"`, "fresh")
		if !s.Ready {
			t.Fatal("startup read erased intent")
		}
		// Replay the observed classes/order; list membership is a synthetic fixture,
		// since v2 did not retain loaded-list response IDs or the raw includeTurns shape.
		for i, target := range []string{"A", "B", "A"} {
			id := 10 + i*10
			resume(t, &s, id, target)
			generation := s.Generation
			req(t, &s, fmt.Sprintf(`{"id":%d,"method":"thread/read","params":{"threadId":%q%s}}`, id+1, target, flag))
			listing(t, &s, id+2, `{"data":["A","B","fresh"],"nextCursor":null}`)
			for j, other := range []string{"A", "B", "fresh"} {
				if other == target {
					continue
				}
				req(t, &s, fmt.Sprintf(`{"id":%d,"method":"thread/read","params":{"threadId":%q%s}}`, id+3+j, other, flag))
			}
			if !s.Ready || s.Thread != target || s.Generation != generation {
				t.Fatal("backfill changed accepted intent", s.Binding)
			}
		}
	}
}

func TestReadBudgetIsFiniteGenerationScopedAndNotASelector(t *testing.T) {
	s := newState("e", 1)
	resume(t, &s, 1, "A")
	listing(t, &s, 2, `{"data":["A","B"],"nextCursor":null}`)
	req(t, &s, `{"id":3,"method":"thread/read","params":{"threadId":"B"}}`)
	if !s.Ready || s.Thread != "A" {
		t.Fatal(s.Binding)
	}
	req(t, &s, `{"id":4,"method":"thread/read","params":{"threadId":"B"}}`)
	if s.Ready {
		t.Fatal("budget reused; cached selection could be hidden")
	}
	resume(t, &s, 5, "A")
	req(t, &s, `{"id":6,"method":"thread/read","params":{"threadId":"B"}}`)
	if s.Ready {
		t.Fatal("old generation budget survived")
	}
}

func TestInvalidOrUncorrelatedListsDoNotAuthorizeReads(t *testing.T) {
	for _, body := range []string{`{"data":["A","B"],"nextCursor":"more"}`, `{"data":["B"]}`, `{"data":["A","B","B"]}`, `{"data":[{}]}`} {
		s := newState("e", 1)
		resume(t, &s, 1, "A")
		listing(t, &s, 2, body)
		req(t, &s, `{"id":3,"method":"thread/read","params":{"threadId":"B"}}`)
		if s.Ready {
			t.Fatal("invalid list granted routing exception", body)
		}
	}
	s := newState("e", 1)
	req(t, &s, startA)
	answer(t, &s, "1", "A")
	listing(t, &s, 2, `{"data":["A","B"]}`)
	req(t, &s, `{"id":3,"method":"thread/read","params":{"threadId":"B"}}`)
	if s.Ready {
		t.Fatal("arbitrary list established resume context")
	}
}

func TestReadShapesRemainDistinguishable(t *testing.T) {
	cases := map[string]string{`{"threadId":"A"}`: "omitted", `{"threadId":"A","includeTurns":false}`: "false", `{"threadId":"A","includeTurns":true}`: "true", `{"threadId":"A","includeTurns":null}`: "null", `{"threadId":"A","includeTurns":{"value":false}}`: "object"}
	for params, want := range cases {
		m := metadata(t, `{"id":1,"method":"thread/read","params":`+params+`}`)
		if m.readShape != want {
			t.Fatal(m.readShape, want)
		}
	}
	for _, flag := range []string{"true", "null", `{"value":false}`} {
		s := newState("e", 1)
		resume(t, &s, 1, "A")
		listing(t, &s, 2, `{"data":["A","B"]}`)
		req(t, &s, `{"id":3,"method":"thread/read","params":{"threadId":"B","includeTurns":`+flag+`}}`)
		if s.Ready {
			t.Fatal("non-metadata shape exempted", flag)
		}
	}
}
