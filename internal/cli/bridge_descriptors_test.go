package cli

import (
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/bridge"
)

// The tools are the allowlist, one per allowed command, and nothing else:
// a command added to the allowlist without its place in the order, or with
// positionals the descriptions do not cover, fails here.
func TestTheToolsAreTheAllowlist(t *testing.T) {
	var allowed []string
	for name := range toolFlags {
		allowed = append(allowed, name)
	}
	offered := slices.Clone(toolOrder)
	sort.Strings(allowed)
	sort.Strings(offered)
	if !slices.Equal(allowed, offered) {
		t.Fatalf("the allowlist is %q, the tools offered %q", allowed, offered)
	}
	tools := ToolDescriptors()
	if len(tools) != len(toolOrder) {
		t.Fatalf("%d tools for %d commands", len(tools), len(toolOrder))
	}
	for _, tool := range tools {
		command := findCommand(tool.Name)
		if names := positionalNames(command); len(names) != len(toolPositionals[tool.Name]) {
			t.Errorf("%s has positionals %q and %d descriptions", tool.Name, names, len(toolPositionals[tool.Name]))
		}
		if tool.Summary == "" {
			t.Errorf("%s has no summary", tool.Name)
		}
		for _, param := range tool.Params {
			if param.Description == "" {
				t.Errorf("%s %s has no description", tool.Name, param.Name)
			}
		}
	}
	if !slices.Contains(toolOrder, "list") {
		t.Fatal("list is not offered")
	}
}

// Each descriptor parses back to its command: every parameter, given alone
// beside the positionals the command needs, gives words the table parses as
// that command with that flag or positional.
func TestEachDescriptorParsesBackToItsCommand(t *testing.T) {
	for _, tool := range ToolDescriptors() {
		base := map[string]any{}
		for _, param := range tool.Params {
			if param.Kind == bridge.ParamWord {
				base[param.Name] = "a " + param.Name
			}
		}
		for _, param := range tool.Params {
			arguments := map[string]any{}
			for name, value := range base {
				arguments[name] = value
			}
			switch param.Kind {
			case bridge.ParamSwitch:
				arguments[param.Name] = true
			case bridge.ParamValue:
				arguments[param.Name] = "1"
			}
			raw, _ := json.Marshal(arguments)
			words, err := tool.Words(raw)
			if err != nil {
				t.Fatalf("%s with %s: %v", tool.Name, param.Name, err)
			}
			result, err := parse(words)
			if err != nil {
				t.Fatalf("%s with %s: %q does not parse: %v", tool.Name, param.Name, words, err)
			}
			if result.Call.Command == nil || result.Call.Command.Name != tool.Name {
				t.Fatalf("%q parses as another command", words)
			}
			if param.Kind != bridge.ParamWord {
				if _, given := result.Call.Flags[param.Name]; !given {
					t.Fatalf("%q lost --%s", words, param.Name)
				}
			} else if !slices.Contains(result.Call.Positionals, "a "+param.Name) {
				t.Fatalf("%q lost its %s", words, param.Name)
			}
		}
	}
}

// A call through a descriptor gives the words ToolWords normalizes to, so
// the digest the transport's binding names is the one the child checks.
func TestADescriptorsWordsAreTheNormalizedWords(t *testing.T) {
	tools := ToolDescriptors()
	for _, call := range []struct {
		tool      string
		arguments string
	}{
		{"inbox", `{}`},
		{"inbox", `{"peek":true,"json":true}`},
		{"inbox", `{"message":"8d4ddd85"}`},
		{"send", `{"name":"web","text":"look -- here","notify":true,"wait":"2"}`},
		{"pending", `{"text":"the suite runs"}`},
		{"whoami", `{"json":false}`},
		{"retry", `{"receipt":"0123456789abcdef01234567"}`},
		{"list", `{"json":true}`},
	} {
		tool, ok := bridge.FindTool(tools, call.tool)
		if !ok {
			t.Fatalf("no tool %s", call.tool)
		}
		words, err := tool.Words(json.RawMessage(call.arguments))
		if err != nil {
			t.Fatalf("%s %s: %v", call.tool, call.arguments, err)
		}
		normalized, err := ToolWords(words)
		if err != nil {
			t.Fatalf("%q is refused: %v", words, err)
		}
		if strings.Join(words, "\x00") != strings.Join(normalized, "\x00") {
			t.Fatalf("%s %s gives %q, normalized %q", call.tool, call.arguments, words, normalized)
		}
	}
}
