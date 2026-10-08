package gateway

import (
	"fmt"
	"testing"
)

func armReadPhase(t *testing.T, s *state, phase string) {
	t.Helper()
	resume(t, s, 1, "A")
	switch phase {
	case "pending":
		req(t, s, `{"id":2,"method":"thread/loaded/list","params":{}}`)
	case "partial":
		listing(t, s, 2, `{"data":["A","B","C"]}`)
		req(t, s, `{"id":3,"method":"thread/read","params":{"threadId":"B"}}`)
	}
}

func finishOldList(t *testing.T, s *state, phase string) {
	t.Helper()
	if phase == "pending" {
		raw := `{"id":2,"result":{"data":["A","B","C"]}}`
		s.response(metadata(t, raw), []byte(raw))
	}
	if phase == "armed" {
		listing(t, s, 20, `{"data":["A","B","C"]}`)
	}
}

func TestWorkBoundariesRevokeEveryReadPhaseWithoutChangingPrimary(t *testing.T) {
	methods := []string{"turn/start", "turn/steer", "turn/interrupt", "thread/compact/start", "thread/goal/set", "thread/goal/clear", "thread/goal/get", "thread/name/set", "review/start"}
	for _, method := range methods {
		for _, phase := range []string{"armed", "pending", "partial"} {
			for _, refused := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/refused=%v", method, phase, refused), func(t *testing.T) {
					s := newState("epoch", 1)
					armReadPhase(t, &s, phase)
					old := s.Binding
					serial := s.readSerial
					req(t, &s, fmt.Sprintf(`{"id":10,"method":%q,"params":{"threadId":"A"}}`, method))
					if s.Binding != old || s.readSerial == serial || s.readPhase() != "closed" {
						t.Fatal("boundary changed primary or retained permission", s.Binding, s.readPhase())
					}
					reply := `{"id":10,"result":{}}`
					if refused {
						reply = `{"id":10,"error":{"code":-32600}}`
					}
					s.response(metadata(t, reply))
					finishOldList(t, &s, phase)
					if s.canBackfill || len(s.backfill) > 0 {
						t.Fatal("late/failed control reopened permission")
					}
					req(t, &s, `{"id":30,"method":"thread/read","params":{"threadId":"C"}}`)
					if s.Ready {
						t.Fatal("stale cohort hid an unrelated read")
					}
				})
			}
		}
	}
}

func TestBoundaryDuringPendingResumeCannotArmOnLateAcceptance(t *testing.T) {
	s := newState("epoch", 1)
	req(t, &s, `{"id":1,"method":"thread/resume","params":{"threadId":"A","config":{},"runtimeWorkspaceRoots":[]}}`)
	req(t, &s, `{"id":2,"method":"turn/interrupt","params":{"threadId":"A"}}`)
	answer(t, &s, "1", "A")
	if !s.Ready || s.canBackfill {
		t.Fatal("late accepted intent rearmed read scope", s.Binding)
	}
	listing(t, &s, 3, `{"data":["A","B"]}`)
	req(t, &s, `{"id":4,"method":"thread/read","params":{"threadId":"B"}}`)
	if s.Ready {
		t.Fatal("boundary bypassed by late resume")
	}
}

func TestResumeMetadataAndOldUnsubscribeAreNotWorkBoundaries(t *testing.T) {
	s := newState("epoch", 1)
	resume(t, &s, 1, "A")
	serial := s.readSerial
	for i, method := range []string{"thread/read", "thread/turns/list", "thread/items/list", "model/list", "config/read"} {
		req(t, &s, fmt.Sprintf(`{"id":%d,"method":%q,"params":{"threadId":"A"}}`, 10+i, method))
	}
	req(t, &s, `{"id":25,"method":"thread/unsubscribe","params":{"threadId":"old"}}`)
	if s.readSerial != serial || !s.canBackfill {
		t.Fatal("resume metadata or teardown lost arming")
	}
	listing(t, &s, 30, `{"data":["A","B"]}`)
	req(t, &s, `{"id":31,"method":"thread/read","params":{"threadId":"B"}}`)
	if !s.Ready {
		t.Fatal("ordinary backfill broke")
	}
}
