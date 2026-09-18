package gateway

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// Only ordering/classes/aliases come from the immutable capture. V2 did not log
// includeTurns shape or loaded-list data; those values are explicit test variants.
func TestOwnerV2MetadataOrderWithBothSupportedFalseEncodings(t *testing.T) {
	raw, e := os.ReadFile("testdata/v2-metadata-order.json")
	if e != nil {
		t.Fatal(e)
	}
	var rows []struct {
		Direction, Method, IDClass, RequestTag, Thread, Source    string
		ConfigPresent, RootsPresent, DirectKnown, Direct, Refused bool
	}
	if e = json.Unmarshal(raw, &rows); e != nil {
		t.Fatal(e)
	}
	for _, omit := range []bool{true, false} {
		s := newState("replay", 1)
		ids := map[string]any{}
		methods := map[string]string{}
		next := 0
		accepted := 0
		for _, r := range rows {
			if r.RequestTag == "" {
				continue
			}
			id, ok := ids[r.RequestTag]
			if !ok {
				next++
				switch r.IDClass {
				case "numeric":
					id = next
				case "uuid":
					id = fmt.Sprintf("00000000-0000-4000-8000-%012d", next)
				case "startup":
					id = fmt.Sprintf("startup-thread-start-%d", next)
				default:
					id = fmt.Sprintf("other-%d", next)
				}
				ids[r.RequestTag] = id
			}
			envelope := map[string]any{"id": id}
			if r.Direction == "tui-request" {
				methods[r.RequestTag] = r.Method
				envelope["method"] = r.Method
				params := map[string]any{}
				if r.Thread != "" {
					params["threadId"] = r.Thread
				}
				if r.Source != "" {
					params["threadSource"] = r.Source
				}
				if r.ConfigPresent {
					params["config"] = map[string]any{}
				}
				if r.RootsPresent {
					params["runtimeWorkspaceRoots"] = []string{}
				}
				if r.Method == "thread/read" && !omit {
					params["includeTurns"] = false
				}
				envelope["params"] = params
				b, _ := json.Marshal(envelope)
				before := s.Generation
				req(t, &s, string(b))
				if r.Method == "thread/read" && s.Generation != before {
					t.Fatalf("read erased intent: class=%s target=%s shapeOmitted=%v", r.IDClass, r.Thread, omit)
				}
			} else {
				if r.Refused {
					envelope["error"] = map[string]any{"code": -1}
				} else {
					result := map[string]any{}
					if r.Thread != "" {
						thread := map[string]any{"id": r.Thread}
						if r.DirectKnown {
							thread["canAcceptDirectInput"] = r.Direct
						}
						result["thread"] = thread
					}
					if methods[r.RequestTag] == "thread/loaded/list" {
						result["data"] = []string{"A", "B", "created-1", "created-2"}
					}
					envelope["result"] = result
				}
				b, _ := json.Marshal(envelope)
				s.response(metadata(t, string(b)), b)
				if (methods[r.RequestTag] == "thread/start" || methods[r.RequestTag] == "thread/resume") && !r.Refused && r.Thread != "" && r.DirectKnown && r.Direct {
					if !s.Ready || s.Thread != r.Thread {
						t.Fatal("accepted intent unavailable after replay", s.Binding)
					}
					accepted++
				}
			}
		}
		if accepted != 5 {
			t.Fatal("unexpected accepted intent count", accepted)
		}
		// The same root read outside the exhausted resume context is ambiguous. It is
		// not evidence of a switch, nor grounds for silently keeping routing available.
		resume(t, &s, 999, "A")
		req(t, &s, `{"id":1000,"method":"thread/read","params":{"threadId":"B"}}`)
		if s.Ready || !strings.Contains(s.Reason, "selection not proved") {
			t.Fatal(s.Binding)
		}
	}
}
