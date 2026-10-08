package workflow

// What the shim requires of a delivery, checked directly.
//
// Split from codexshim_contract_test.go when that file passed the project's
// 400-line limit: the handshake and the conversation are one subject, and what
// a turn/start must look like is another. Every case here decides and returns
// — accepting a turn would start work in a goroutine that outlives the test,
// see the note in the contract file.

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

// A delivery is a turn with a present, empty input list. Both halves matter:
// a decoder reads an absent field and a null one as the same empty slice, so a
// shim that asks only for a length accepts two requests the real server
// refuses — and a fixture that is more forgiving than the server makes every
// scenario built on it weaker.
func TestShimDeliveryRequiresAPresentEmptyInput(t *testing.T) {
	notice := `{"notice":"one message","members":[{"id":"m1","from":"sender","fromEpoch":"e1","to":"me","toEpoch":"e2"}]}`
	delivery := func(input string) json.RawMessage {
		return json.RawMessage(`{"threadId":"` + shimThread + `","clientUserMessageId":"c1",` + input +
			`"toolOutput":{"name":"` + mailboxToolName + `","output":` + strconv.Quote(notice) + `}}`)
	}
	t.Setenv(sessionNameEnv, "me")
	t.Setenv(sessionEpochEnv, "e2")
	for _, tc := range []struct {
		name, input string
		accepted    bool
	}{
		{"empty-list", `"input":[],`, true},
		{"absent", ``, false},
		{"null", `"input":null,`, false},
		{"a-string", `"input":"",`, false},
		{"with-text", `"input":[{"type":"text","text":"hello"}],`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The check, not the whole turn: accepting one starts the work in a
			// goroutine that outlives this test, and that goroutine runs rewake
			// with whatever environment is current when it gets there — which,
			// once t.Setenv has restored, is the environment of a live session.
			s := &shimSession{thread: shimThread}
			notice, err := s.checkedDelivery(delivery(tc.input))
			if tc.accepted && err != nil {
				t.Fatalf("a valid delivery was refused: %v", err)
			}
			if !tc.accepted && err == nil {
				t.Fatalf("an invalid delivery was accepted: %#v", notice)
			}
		})
	}
}

// A protocol field with a wrong value is not an unknown extension: the real
// server refuses `effort: 5` and `model: true`, and a fixture that decodes only
// the fields it cares about accepts both. What a delivery may carry is a closed
// table here, so anything else is refused by name.
//
// This is the half of the check that needs no Codex installed; the other half
// compares the same verdicts against the generated schema.
func TestShimDeliveryRefusesFieldsItDoesNotServe(t *testing.T) {
	t.Setenv(sessionNameEnv, "me")
	t.Setenv(sessionEpochEnv, "e2")
	notice := `{"notice":"one message","members":[{"id":"m1","from":"sender","fromEpoch":"e1","to":"me","toEpoch":"e2"}]}`
	output := `"toolOutput":{"name":"` + mailboxToolName + `","output":` + strconv.Quote(notice) + `}`
	for _, tc := range []struct{ name, extra string }{
		{"effort-as-a-number", `"effort":5,`},
		{"model-as-a-boolean", `"model":true,`},
		{"a-field-we-do-not-serve", `"personality":"friendly",`},
		{"thread-id-as-a-number", `"threadId":5,` + `"clientUserMessageId":"c1",`},
		{"a-root-that-is-not-a-path", `"runtimeWorkspaceRoots":[7],`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := `{"threadId":"` + shimThread + `","clientUserMessageId":"c1","input":[],` + tc.extra + output + `}`
			s := &shimSession{thread: shimThread}
			if notice, err := s.checkedDelivery(json.RawMessage(params)); err == nil {
				t.Fatalf("a delivery carrying %s was accepted: %#v", tc.extra, notice)
			}
		})
	}
}

// The same question one level down, which is where the check used to stop. A
// nested object that only had to *be* an object let every field inside it
// through: `toolOutput.namespace` is a real protocol field, and the server
// refuses all four of these values for it.
func TestShimDeliveryChecksNestedObjects(t *testing.T) {
	t.Setenv(sessionNameEnv, "me")
	t.Setenv(sessionEpochEnv, "e2")
	member := `{"id":"m1","from":"sender","fromEpoch":"e1","to":"me","toEpoch":"e2"}`
	for _, tc := range []struct{ name, output string }{
		{"namespace-as-a-boolean", `"toolOutput":{"name":"` + mailboxToolName + `","namespace":true,"output":` +
			strconv.Quote(`{"notice":"one","members":[`+member+`]}`) + `}`},
		{"namespace-as-a-list", `"toolOutput":{"name":"` + mailboxToolName + `","namespace":[1],"output":` +
			strconv.Quote(`{"notice":"one","members":[`+member+`]}`) + `}`},
		{"an-output-field-we-do-not-serve", `"toolOutput":{"name":"` + mailboxToolName + `","callId":"x","output":` +
			strconv.Quote(`{"notice":"one","members":[`+member+`]}`) + `}`},
		{"a-notice-field-we-do-not-serve", `"toolOutput":{"name":"` + mailboxToolName + `","output":` +
			strconv.Quote(`{"notice":"one","preview":"x","members":[`+member+`]}`) + `}`},
		{"a-member-field-we-do-not-serve", `"toolOutput":{"name":"` + mailboxToolName + `","output":` +
			strconv.Quote(`{"notice":"one","members":[{"id":"m1","from":"sender","fromEpoch":"e1","to":"me","toEpoch":"e2","kind":"task"}]}`) + `}`},
		{"a-member-id-that-is-a-number", `"toolOutput":{"name":"` + mailboxToolName + `","output":` +
			strconv.Quote(`{"notice":"one","members":[{"id":7,"from":"sender","fromEpoch":"e1","to":"me","toEpoch":"e2"}]}`) + `}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := `{"threadId":"` + shimThread + `","clientUserMessageId":"c1","input":[],` + tc.output + `}`
			s := &shimSession{thread: shimThread}
			if notice, err := s.checkedDelivery(json.RawMessage(params)); err == nil {
				t.Fatalf("a delivery carrying %s was accepted: %#v", tc.name, notice)
			}
		})
	}
}

// A string is not automatically a path, and a path is not automatically one
// the server takes: 0.155.1 refuses a relative workspace root outright. A
// description that stopped at "it is a string" accepted one.
func TestShimRefusesARelativeRoot(t *testing.T) {
	t.Setenv(sessionNameEnv, "me")
	t.Setenv(sessionEpochEnv, "e2")
	notice := `{"notice":"one message","members":[{"id":"m1","from":"sender","fromEpoch":"e1","to":"me","toEpoch":"e2"}]}`
	output := `"toolOutput":{"name":"` + mailboxToolName + `","output":` + strconv.Quote(notice) + `}`
	for _, root := range []string{`"work"`, `"./work"`, `"../work"`, `""`} {
		t.Run(root, func(t *testing.T) {
			params := `{"threadId":"` + shimThread + `","clientUserMessageId":"c1","input":[],` +
				`"runtimeWorkspaceRoots":[` + root + `],` + output + `}`
			s := &shimSession{thread: shimThread}
			if notice, err := s.checkedDelivery(json.RawMessage(params)); err == nil {
				t.Fatalf("a delivery with root %s was accepted: %#v", root, notice)
			}
		})
	}
	params := `{"threadId":"` + shimThread + `","clientUserMessageId":"c1","input":[],` +
		`"runtimeWorkspaceRoots":["/work"],` + output + `}`
	s := &shimSession{thread: shimThread}
	if _, err := s.checkedDelivery(json.RawMessage(params)); err != nil {
		t.Fatalf("an absolute root was refused: %v", err)
	}
}

// A name that appears twice has no single value, and readers of it disagree:
// decoding into a struct kept the earlier object's fields while a map kept the
// later one's, so a request missing a required field passed as complete. The
// fixture refuses the repetition itself, which is the only answer that cannot
// depend on which reader looks.
func TestShimRefusesRepeatedKeys(t *testing.T) {
	t.Setenv(sessionNameEnv, "me")
	t.Setenv(sessionEpochEnv, "e2")
	notice := `{"notice":"one message","members":[{"id":"m1","from":"sender","fromEpoch":"e1","to":"me","toEpoch":"e2"}]}`
	output := `"toolOutput":{"name":"` + mailboxToolName + `","output":` + strconv.Quote(notice) + `}`
	for _, tc := range []struct{ name, params string }{
		{"tool-output-twice", `{"threadId":"` + shimThread + `","clientUserMessageId":"c1","input":[],` +
			output + `,"toolOutput":{}}`},
		{"thread-id-twice", `{"threadId":"` + shimThread + `","threadId":"other","clientUserMessageId":"c1",` +
			`"input":[],` + output + `}`},
		{"a-notice-field-twice", `{"threadId":"` + shimThread + `","clientUserMessageId":"c1","input":[],` +
			`"toolOutput":{"name":"` + mailboxToolName + `","output":` +
			strconv.Quote(`{"notice":"one","notice":"two","members":[{"id":"m1","from":"s","fromEpoch":"e1","to":"me","toEpoch":"e2"}]}`) + `}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &shimSession{thread: shimThread}
			notice, err := s.checkedDelivery(json.RawMessage(tc.params))
			if err == nil {
				t.Fatalf("a delivery repeating a key was accepted: %#v", notice)
			}
			// The reason matters: a neighboring refusal would make this test
			// green while repetition went unnoticed, which is how the first
			// version of it passed.
			if !strings.Contains(err.Error(), "twice") {
				t.Fatalf("refused for the wrong reason: %v", err)
			}
		})
	}
}

// A number too large for a float used to end the scan for repeated names in
// the middle, and the scan reported "nothing repeated" — so a request could
// carry both and pass. The scan now says it could not finish, which is a
// refusal like any other.
func TestShimRefusesWhatItCannotScan(t *testing.T) {
	t.Setenv(sessionNameEnv, "me")
	t.Setenv(sessionEpochEnv, "e2")
	notice := `{"notice":"one message","members":[{"id":"m1","from":"sender","fromEpoch":"e1","to":"me","toEpoch":"e2"}]}`
	output := `"toolOutput":{"name":"` + mailboxToolName + `","output":` + strconv.Quote(notice) + `}`
	// The number comes first and the repetition after it, so a scan that stops
	// at the number never reaches the repeated name — which is exactly how
	// this request passed before.
	huge := strings.Repeat("9", 400)
	params := `{"threadId":"` + shimThread + `","clientUserMessageId":"c1","input":[],` +
		`"toolOutput":{"name":"` + mailboxToolName + `","output":` + strconv.Quote(notice) + `,"huge":` + huge + `},` +
		output + `}`
	s := &shimSession{thread: shimThread}
	delivered, err := s.checkedDelivery(json.RawMessage(params))
	if err == nil {
		t.Fatalf("a delivery the fixture could not scan was accepted: %#v", delivered)
	}
	// Refusing because of the repetition is the right answer; refusing because
	// the scan could not finish is also right. Passing is not, and neither is
	// a refusal that names something else, because that would mean the scan
	// went quiet again.
	if !strings.Contains(err.Error(), "twice") && !strings.Contains(err.Error(), "could not be read to the end") {
		t.Fatalf("refused for the wrong reason: %v", err)
	}
}

// A repeated name may itself be the empty string, and a scan that reports its
// find by returning the name has no way to say so: emptiness already means
// "nothing found". The pair of empty names below ends the scan, and the real
// repetition after them goes unnoticed — until the find is a separate answer.
func TestShimRefusesARepeatedEmptyName(t *testing.T) {
	t.Setenv(sessionNameEnv, "me")
	t.Setenv(sessionEpochEnv, "e2")
	notice := `{"notice":"one message","members":[{"id":"m1","from":"sender","fromEpoch":"e1","to":"me","toEpoch":"e2"}]}`
	output := `"toolOutput":{"name":"` + mailboxToolName + `","output":` + strconv.Quote(notice) + `}`
	params := `{"":1,"":2,"threadId":"` + shimThread + `","threadId":"other","clientUserMessageId":"c1","input":[],` +
		output + `}`
	s := &shimSession{thread: shimThread}
	delivered, err := s.checkedDelivery(json.RawMessage(params))
	if err == nil {
		t.Fatalf("a delivery repeating an empty name was accepted: %#v", delivered)
	}
	if !strings.Contains(err.Error(), "twice") {
		t.Fatalf("refused for the wrong reason: %v", err)
	}
}

// A document that ends inside an object has not been read to the end, and a
// scan that reports otherwise is promising more than it did — even where the
// description refuses the request anyway.
func TestShimScanRefusesATruncatedValue(t *testing.T) {
	if problem := singleNames("the delivery", json.RawMessage(`{"a":1,"b":{`)); problem == "" {
		t.Fatal("a truncated object was reported as fully scanned")
	}
	if problem := singleNames("the delivery", json.RawMessage(`{"a":1,"b":2}`)); problem != "" {
		t.Fatalf("a complete object was refused: %s", problem)
	}
}

// A field the description requires and the request does not carry.
func TestShimRequiresTheFieldsADeliveryMustCarry(t *testing.T) {
	t.Setenv(sessionNameEnv, "me")
	t.Setenv(sessionEpochEnv, "e2")
	notice := `{"notice":"one message","members":[{"id":"m1","from":"sender","fromEpoch":"e1","to":"me","toEpoch":"e2"}]}`
	output := `"toolOutput":{"name":"` + mailboxToolName + `","output":` + strconv.Quote(notice) + `}`
	for _, tc := range []struct{ name, params string }{
		{"no-tool-output", `{"threadId":"` + shimThread + `","clientUserMessageId":"c1","input":[]}`},
		{"no-message-id", `{"threadId":"` + shimThread + `","input":[],` + output + `}`},
		{"no-output-in-the-tool-output", `{"threadId":"` + shimThread + `","clientUserMessageId":"c1","input":[],` +
			`"toolOutput":{"name":"` + mailboxToolName + `"}}`},
		{"a-member-without-an-epoch", `{"threadId":"` + shimThread + `","clientUserMessageId":"c1","input":[],` +
			`"toolOutput":{"name":"` + mailboxToolName + `","output":` +
			strconv.Quote(`{"notice":"one","members":[{"id":"m1","from":"s","to":"me","toEpoch":"e2"}]}`) + `}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &shimSession{thread: shimThread}
			if notice, err := s.checkedDelivery(json.RawMessage(tc.params)); err == nil {
				t.Fatalf("a delivery missing a required field was accepted: %#v", notice)
			}
		})
	}
}

// And the conversation requests, which carry a nested object of their own.
func TestShimConversationRequestsCheckTheirNesting(t *testing.T) {
	for _, tc := range []struct{ name, params string }{
		{"a-setting-we-do-not-serve", `{"threadSource":"user","config":{"model":"x"},"runtimeWorkspaceRoots":["/work"]}`},
		{"config-as-a-list", `{"threadSource":"user","config":[],"runtimeWorkspaceRoots":["/work"]}`},
		{"a-root-that-is-not-a-path", `{"threadSource":"user","config":{},"runtimeWorkspaceRoots":[7]}`},
		{"a-start-field-we-do-not-serve", `{"threadSource":"user","config":{},"cwd":"/work","runtimeWorkspaceRoots":["/work"]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &shimSession{thread: shimThread}
			peer := &shimPeer{initialized: true, experimental: true}
			if result, _, err := s.answer(peer, "thread/start", json.RawMessage(tc.params)); err == nil {
				t.Fatalf("a start carrying %s was accepted: %#v", tc.name, result)
			}
		})
	}
}

// Mail addressed to another session, or to an earlier run of this one, is not
// this session's to answer. The real server cannot know that; the shim can,
// because the wrapper tells it which session it is running.
func TestShimDeliveryMustBeAddressedHere(t *testing.T) {
	t.Setenv(sessionNameEnv, "me")
	t.Setenv(sessionEpochEnv, "e2")
	for _, tc := range []struct{ name, to, epoch string }{
		{"another-session", "somebody-else", "e2"},
		{"an-earlier-run", "me", "e1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			notice := `{"notice":"one message","members":[{"id":"m1","from":"sender","fromEpoch":"e1","to":"` +
				tc.to + `","toEpoch":"` + tc.epoch + `"}]}`
			params := json.RawMessage(`{"threadId":"` + shimThread + `","clientUserMessageId":"c1","input":[],` +
				`"toolOutput":{"name":"` + mailboxToolName + `","output":` + strconv.Quote(notice) + `}}`)
			s := &shimSession{thread: shimThread}
			if notice, err := s.checkedDelivery(params); err == nil {
				t.Fatalf("a delivery for %s/%s was accepted: %#v", tc.to, tc.epoch, notice)
			}
		})
	}
}
