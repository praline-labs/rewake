package modhost

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"
)

// The mod's wire is pinned word by word (brief 94): what it sends up is a
// reading of Claude Code, an outcome of a native call or a carried event,
// and what the wrapper makes of them is Go's. A new endpoint, field or code
// fails here until it is listed; a reading the wrapper would project goes
// raw inside a native field rather than as a field of its own. The digest
// pin (protocol_pin_test.go) holds the version; this one says what changed.

// modSends is each endpoint the mod posts to, by its request. status/line is
// not the mod's: rewake statusline posts it.
var modSends = map[string]string{
	"hello":           "HelloRequest",
	"turn/start":      "TurnStartRequest",
	"turn/end":        "TurnEndRequest",
	"mod/alive":       "AliveRequest",
	"mod/ended":       "EndedRequest",
	"mod/state":       "StateRequest",
	"mod/control":     "ControlReply",
	"mirror/events":   "MirrorBatch",
	"tool/plan":       "ToolPlanRequest",
	"tool/result":     "ToolResultRequest",
	"tool/native":     "ToolNativeRequest",
	"session/closing": "ClosingRequest",
	"ask/open":        "AskOpenRequest",
	"ask/poll":        "AskPollRequest",
	"ask/settle":      "AskSettleRequest",
}

var modSays = map[string]string{
	"AliveRequest":      "generation loadId toolsDigest work",
	"AskKind":           "permission question",
	"AskOpenRequest":    "cwd input kind loadId questions readOnly tool",
	"AskOption":         "description label",
	"AskPollRequest":    "id loadId",
	"AskQuestion":       "header multiSelect options question",
	"AskSettleRequest":  "id loadId winner",
	"AskWinner":         "main local expired aborted",
	"ClosingRequest":    "loadId reason",
	"ControlCode":       "conversation busy turn idle",
	"ControlReply":      "code generation id loadId native outcome",
	"EndReason":         "answered aborted refused failed",
	"EndedRequest":      "epochs loadId",
	"HelloRequest":      "enter harnessVersion loadId loadSeq protocol sessionId",
	"MirrorBatch":       "events generation loadId",
	"MirrorBlock":       "text thinking",
	"MirrorEnd":         "done error refused",
	"MirrorEvent":       "block id kind level lost mode name of origin part permit retry state status text turn",
	"MirrorEventKind":   "prompt delta settle tool input result turn mode lost resync",
	"MirrorPermit":      "allowed by reason",
	"MirrorPermitBy":    "rule mode classifier person rewake hook",
	"MirrorTurn":        "working idle compacting",
	"NativeCall":        "deny error isError text",
	"StateRequest":      "activity compacting compactions conversation dialog from generation loadId native request sequence turn waiting",
	"ToolNativeRequest": "inside loadId made moved path tool",
	"ToolPlanRequest":   "arguments loadId tool",
	"ToolRan":           "error exitCode stderr stdout stdoutCut",
	"ToolResultRequest": "loadId ran tool",
	"TurnEndRequest":    "answer conversation endId loadId reason turnId",
	"TurnStartRequest":  "conversation loadId turnId",
}

// upWords are the endpoints a schema's mod posts to and the words of every
// definition their requests reach: a definition's fields, or an enum's
// values.
func upWords(schema []byte) (map[string]string, map[string]string, error) {
	var s struct {
		Endpoints []struct {
			Path    string `json:"path"`
			Request struct {
				Ref string `json:"$ref"`
			} `json:"request"`
		} `json:"x-endpoints"`
		Defs map[string]json.RawMessage `json:"$defs"`
	}
	if err := json.Unmarshal(schema, &s); err != nil {
		return nil, nil, err
	}
	sends, says := map[string]string{}, map[string]string{}
	var walk func(json.RawMessage) error
	reach := func(ref string) error {
		name := ref[strings.LastIndex(ref, "/")+1:]
		if _, seen := says[name]; seen {
			return nil
		}
		def, ok := s.Defs[name]
		if !ok {
			return fmt.Errorf("no definition %s", name)
		}
		var node struct {
			Properties map[string]json.RawMessage `json:"properties"`
			Enum       []string                   `json:"enum"`
		}
		if err := json.Unmarshal(def, &node); err != nil {
			return err
		}
		words := node.Enum
		if node.Properties != nil {
			words = slices.Sorted(maps.Keys(node.Properties))
		}
		says[name] = strings.Join(words, " ")
		return walk(def)
	}
	walk = func(raw json.RawMessage) error {
		var node struct {
			Ref        string                     `json:"$ref"`
			Properties map[string]json.RawMessage `json:"properties"`
			Items      json.RawMessage            `json:"items"`
			Additional json.RawMessage            `json:"additionalProperties"`
		}
		if err := json.Unmarshal(raw, &node); err != nil {
			return nil // a boolean additionalProperties
		}
		if node.Ref != "" {
			return reach(node.Ref)
		}
		for _, key := range slices.Sorted(maps.Keys(node.Properties)) {
			if err := walk(node.Properties[key]); err != nil {
				return err
			}
		}
		for _, inner := range []json.RawMessage{node.Items, node.Additional} {
			if len(inner) > 0 {
				if err := walk(inner); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, e := range s.Endpoints {
		if e.Path == "status/line" {
			continue
		}
		sends[e.Path] = e.Request.Ref[strings.LastIndex(e.Request.Ref, "/")+1:]
		if err := reach(e.Request.Ref); err != nil {
			return nil, nil, err
		}
	}
	return sends, says, nil
}

// unlisted is how a schema's mod words differ from the pinned ones.
func unlisted(schema []byte) ([]string, error) {
	sends, says, err := upWords(schema)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, path := range slices.Sorted(maps.Keys(sends)) {
		if modSends[path] != sends[path] {
			out = append(out, fmt.Sprintf("the mod posts %s %s, not listed", path, sends[path]))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(says)) {
		if modSays[name] != says[name] {
			out = append(out, fmt.Sprintf("the mod says %s {%s}, listed {%s}: a reading the wrapper projects goes raw inside native", name, says[name], modSays[name]))
		}
	}
	for path := range modSends {
		if _, ok := sends[path]; !ok {
			out = append(out, "the mod no longer posts "+path+": drop it from modSends")
		}
	}
	return out, nil
}

func TestTheModSendsOnlyTheListedWords(t *testing.T) {
	schema, err := os.ReadFile("../mod/protocol.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	got, err := unlisted(schema)
	if err != nil || len(got) != 0 {
		t.Fatalf("%v\n%s", err, strings.Join(got, "\n"))
	}
	// A field the Статус would read, a code of a worded reply and an
	// endpoint of its own are caught.
	seeds := map[string]func(defs map[string]any, endpoints *[]any){
		"a field": func(defs map[string]any, _ *[]any) {
			defs["StateRequest"].(map[string]any)["properties"].(map[string]any)["percent"] = map[string]any{"type": "integer"}
		},
		"a code": func(defs map[string]any, _ *[]any) {
			code := defs["ControlCode"].(map[string]any)
			code["enum"] = append(code["enum"].([]any), "nothing-to-compact")
		},
		"an endpoint": func(_ map[string]any, endpoints *[]any) {
			*endpoints = append(*endpoints, map[string]any{"path": "mod/level", "request": map[string]any{"$ref": "#/$defs/AliveRequest"}})
		},
	}
	for name, seed := range seeds {
		var s map[string]any
		if err := json.Unmarshal(schema, &s); err != nil {
			t.Fatal(err)
		}
		endpoints := s["x-endpoints"].([]any)
		seed(s["$defs"].(map[string]any), &endpoints)
		s["x-endpoints"] = endpoints
		changed, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := unlisted(changed); err != nil || len(got) != 1 {
			t.Errorf("%s: %q, %v", name, got, err)
		}
	}
}
