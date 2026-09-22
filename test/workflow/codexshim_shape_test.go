package workflow

// The shape half of what the shim owes the protocol, kept from the acceptance
// reviews. The checker itself lives in schema_check_test.go; these are the
// cases that caught it claiming more than it checked — a bare string where a
// tagged object belongs, a null conversation, a numeric model name.

import (
	"encoding/json"
	"testing"
)

func TestShimShapeActualResponseShape(t *testing.T) {
	bundle := schemaForTest(t)
	s := &shimSession{thread: shimThread}
	reply := s.startResponse(s.threadDescription(shimThread))
	current := bundle.check("ThreadStartResponse", mustMessage(t, reply))
	if _, ok := reply["sandbox"].(map[string]any); !ok {
		t.Errorf("saved 0.155.1 schema requires a tagged object for sandbox; actual=%#v, checker problems=%v", reply["sandbox"], current)
	}
	reply["thread"] = nil
	reply["model"] = 42
	if problems := bundle.check("ThreadStartResponse", mustMessage(t, reply)); len(problems) == 0 {
		t.Error("checker also accepts null required thread and numeric model")
	}
}

func TestShimShapeRepeatedInitializeIsRejected(t *testing.T) {
	p := &shimPeer{}
	params := []byte(`{"clientInfo":{"name":"review","version":"1"}}`)
	if _, err := p.initialize(params); err != nil {
		t.Fatal(err)
	}
	if _, err := p.initialize(params); err == nil {
		t.Error("second initialize accepted instead of Already initialized")
	}
}

func TestShimShapeInvalidRequestsDoNotGetSuccess(t *testing.T) {
	cases := []struct{ method, params string }{
		{"thread/start", `[]`},
		{"thread/start", `{"runtimeWorkspaceRoots":"/work"}`},
		{"thread/settings/update", `{}`},
		{"thread/metadata/update", `{}`},
		{"thread/unsubscribe", `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.method+tc.params, func(t *testing.T) {
			s := &shimSession{thread: shimThread}
			p := &shimPeer{initialized: true, experimental: true}
			result, _, err := s.answer(p, tc.method, json.RawMessage(tc.params))
			if err == nil {
				t.Errorf("invalid request accepted: %s %s; result=%#v", tc.method, tc.params, result)
			}
		})
	}
}

func mustMessage(t *testing.T, value any) map[string]any {
	t.Helper()
	message, err := asMessage(value)
	if err != nil {
		t.Fatal(err)
	}
	return message
}

// schemaForTest gives a direct test the same bundle the scenario uses, or
// skips it when there is no Codex and none was named: without one there is
// nothing to check against. A Codex that was there and failed is a failure,
// not a skip.
func schemaForTest(t *testing.T) *schemaBundle {
	t.Helper()
	// Under the switch: without it these tests would run a real Codex — the
	// installed one, with the owner's HOME — on every go test ./..., which is
	// exactly what the five ordinary checks must not do.
	if !suite.enabled {
		t.Skipf("needs a real Codex: set %s=1 to run it with the workflow suite", switchEnv)
	}
	rec := &recorder{}
	c := newCase(rec, Spec{Name: "schema-for-test", Observations: []string{"a"}})
	c.Observed("a", "not a scenario")
	defer rec.finish()
	bundle, source, err := loadSchema(c, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if bundle == nil {
		t.Skip(source.absent)
	}
	return bundle
}
