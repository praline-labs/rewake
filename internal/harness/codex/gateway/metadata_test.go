package gateway

import (
	"testing"
)

func TestOpaqueValuesAndExactRequestIDs(t *testing.T) {
	raw := ` {"id":"1","method":"thread/resume","params":{"threadId":"A","config":{"nested":"SECRET\\\"","runtimeWorkspaceRoots":"not metadata"},"runtimeWorkspaceRoots":[],"history":[{"threadId":"wrong","text":"SECRET"}]}} `
	m, e := project([]byte(raw))
	if e != nil || m.id != "s:1" || m.thread != "A" || !m.roots || !m.config {
		t.Fatalf("%+v %v", m, e)
	}
	n := metadata(t, `{"id":1,"method":"thread/list"}`)
	if m.id == n.id {
		t.Fatal("string/numeric ids collided")
	}
	for _, bad := range []string{`{"id":1,"id":2}`, `{"params":{"threadId":"A","threadId":"B"}}`, `{"id":1.5}`, `[{}]`, `{"id":null}`, `{"result":{"data":[],"data":["A"]}}`, `{"params":{"includeTurns":false,"includeTurns":true}}`} {
		if _, e = project([]byte(bad)); e == nil {
			t.Fatal("accepted ambiguous envelope", bad)
		}
	}
	if _, e = project(make([]byte, maxMessage+1)); e == nil {
		t.Fatal("unbounded message")
	}
}
