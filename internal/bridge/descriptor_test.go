package bridge

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

var sendTool = ToolDescriptor{Name: "send", Summary: "tell a session", Params: []ToolParam{
	{Name: "notify", Kind: ParamSwitch, Description: "a heads-up"},
	{Name: "wait", Kind: ParamValue, Description: "seconds"},
	{Name: "json", Kind: ParamSwitch, Description: "as JSON"},
	{Name: "name", Kind: ParamWord, Description: "the session", Required: true},
	{Name: "text", Kind: ParamWord, Description: "the text", Required: true},
}}

// The schema declares the parameters and nothing more.
func TestAToolsSchemaIsItsParameters(t *testing.T) {
	schema := sendTool.Schema()
	properties := schema["properties"].(map[string]any)
	if len(properties) != len(sendTool.Params) || schema["additionalProperties"] != false {
		t.Fatalf("the schema: %v", schema)
	}
	if kind := properties["notify"].(map[string]any)["type"]; kind != "boolean" {
		t.Fatalf("a switch is %v", kind)
	}
	if kind := properties["wait"].(map[string]any)["type"]; kind != "string" {
		t.Fatalf("a value is %v", kind)
	}
	if required := schema["required"].([]string); !slices.Equal(required, []string{"name", "text"}) {
		t.Fatalf("required: %q", required)
	}
}

// A call's arguments become the command's words in the CLI's normal form,
// and anything the descriptor does not offer is refused.
func TestACallsArgumentsBecomeTheCommandsWords(t *testing.T) {
	words, err := sendTool.Words(json.RawMessage(`{"wait":"2","text":"look -- here","notify":true,"name":"web","json":false}`))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"send", "--notify", "--wait=2", "--", "web", "look -- here"}; !slices.Equal(words, want) {
		t.Fatalf("%q, want %q", words, want)
	}
	if words, err := (ToolDescriptor{Name: "whoami"}).Words(nil); err != nil || !slices.Equal(words, []string{"whoami"}) {
		t.Fatalf("no arguments: %q %v", words, err)
	}
	for _, refused := range []struct{ arguments, says string }{
		{`["send"]`, "not one object"},
		{`{"name":"web","text":"x","to":"id"}`, `no "to"`},
		{`{"name":"web","text":"x","notify":"yes"}`, "true or false"},
		{`{"name":"web","text":"x","wait":2}`, "a string"},
		{`{"name":"web"}`, "needs text"},
		{`{"text":"x"}`, "without name"},
		{`{"name":"web","text":"` + strings.Repeat("x", maxArguments) + `"}`, "longer than"},
	} {
		if _, err := sendTool.Words(json.RawMessage(refused.arguments)); err == nil || !strings.Contains(err.Error(), refused.says) {
			t.Errorf("%.60s: %v, want %q", refused.arguments, err, refused.says)
		}
	}
}

// The digest of a set names the set: another order, another summary, another
// parameter is another set.
func TestTheDigestNamesTheSet(t *testing.T) {
	whoami := ToolDescriptor{Name: "whoami", Summary: "who"}
	base := DescriptorsDigest([]ToolDescriptor{sendTool, whoami})
	if base != DescriptorsDigest([]ToolDescriptor{sendTool, whoami}) {
		t.Fatal("one set digests two ways")
	}
	changed := whoami
	changed.Summary = "who this is"
	for _, other := range [][]ToolDescriptor{{whoami, sendTool}, {sendTool, changed}, {sendTool}} {
		if DescriptorsDigest(other) == base {
			t.Fatalf("%v digests as the set", other)
		}
	}
	if tool, ok := FindTool([]ToolDescriptor{sendTool, whoami}, "whoami"); !ok || tool.Summary != "who" {
		t.Fatal("whoami is not found")
	}
}
