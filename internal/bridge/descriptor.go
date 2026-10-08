package bridge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// A tool is one command of the CLI offered to a model
// (docs/v2/design-api.md#tooltransport). The CLI builds a descriptor per
// command its allowlist lets through, from the same table it parses with; what
// follows from a descriptor alone — the schema a harness registers, the
// command's words for a call's arguments, the digest of the whole set — is
// here, so that every transport derives it alike and none imports the CLI.

// ToolDescriptor is one tool: the command it runs, what it does, and what a
// call may give it.
type ToolDescriptor struct {
	// Name is the command's word, and the tool's name.
	Name    string      `json:"name"`
	Summary string      `json:"summary"`
	Params  []ToolParam `json:"params,omitempty"`
}

// ToolParam is a flag or a positional of the command.
type ToolParam struct {
	// Name is the flag without its dashes, or the positional's name.
	Name string `json:"name"`
	// Kind is how the call gives it: ParamSwitch, ParamValue or ParamWord.
	Kind        string `json:"kind"`
	Description string `json:"description"`
	// Required marks a positional the command does not run without.
	Required bool `json:"required,omitempty"`
}

// The kinds of a parameter.
const (
	// ParamSwitch is a flag that takes no value: true gives it.
	ParamSwitch = "switch"
	// ParamValue is a flag with a value.
	ParamValue = "value"
	// ParamWord is a positional, in the order the descriptor lists them.
	ParamWord = "word"
)

// maxArguments bounds a call's arguments before they are decoded: the most
// words a call may carry, each quoted, with room for its keys.
const maxArguments = MaxWordsBytes + 4096

// Schema is the JSON Schema of a call's arguments: one property per
// parameter, nothing else.
func (d ToolDescriptor) Schema() map[string]any {
	properties := map[string]any{}
	var required []string
	for _, param := range d.Params {
		kind := "string"
		if param.Kind == ParamSwitch {
			kind = "boolean"
		}
		properties[param.Name] = map[string]any{"type": kind, "description": param.Description}
		if param.Required {
			required = append(required, param.Name)
		}
	}
	schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// Words turns a call's arguments into the command's words, in the form the
// CLI normalizes to: the command, its flags sorted with their values inline,
// then its positionals after --. The arguments are bounded before they are
// decoded, and anything the descriptor does not name is refused: what runs is
// what the schema offered, and the CLI checks the words again.
func (d ToolDescriptor) Words(arguments json.RawMessage) ([]string, error) {
	if len(arguments) > maxArguments {
		return nil, fmt.Errorf("the arguments are longer than %d bytes", maxArguments)
	}
	given := map[string]json.RawMessage{}
	if trimmed := bytes.TrimSpace(arguments); len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
		if err := json.Unmarshal(trimmed, &given); err != nil {
			return nil, errors.New("the arguments are not one object")
		}
	}
	for name := range given {
		if _, ok := d.param(name); !ok {
			return nil, fmt.Errorf("%s takes no %q", d.Name, name)
		}
	}
	var flags []flagWord
	var positionals []string
	missing := ""
	for _, param := range d.Params {
		raw, ok := given[param.Name]
		if !ok {
			if param.Kind == ParamWord && missing == "" {
				missing = param.Name
			}
			continue
		}
		switch param.Kind {
		case ParamSwitch:
			var on bool
			if err := json.Unmarshal(raw, &on); err != nil {
				return nil, fmt.Errorf("%s is true or false", param.Name)
			}
			if on {
				flags = append(flags, flagWord{param.Name, "--" + param.Name})
			}
		case ParamValue, ParamWord:
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				return nil, fmt.Errorf("%s is a string", param.Name)
			}
			if param.Kind == ParamValue {
				flags = append(flags, flagWord{param.Name, "--" + param.Name + "=" + value})
				continue
			}
			if missing != "" {
				return nil, fmt.Errorf("%s is given without %s before it", param.Name, missing)
			}
			positionals = append(positionals, value)
		default:
			return nil, fmt.Errorf("%s is of a kind no call gives", param.Name)
		}
	}
	for _, param := range d.Params {
		if _, ok := given[param.Name]; param.Required && !ok {
			return nil, fmt.Errorf("%s needs %s", d.Name, param.Name)
		}
	}
	sort.Slice(flags, func(i, j int) bool { return flags[i].name < flags[j].name })
	words := []string{d.Name}
	for _, flag := range flags {
		words = append(words, flag.word)
	}
	if len(positionals) > 0 {
		words = append(append(words, "--"), positionals...)
	}
	return words, nil
}

// flagWord is a flag's word, kept with its name: the CLI sorts flags by name.
type flagWord struct{ name, word string }

func (d ToolDescriptor) param(name string) (ToolParam, bool) {
	for _, param := range d.Params {
		if param.Name == name {
			return param, true
		}
	}
	return ToolParam{}, false
}

// DescriptorsDigest names a set of tools: a transport answers it for the set
// it registered, and the host compares it with the set it offered.
func DescriptorsDigest(tools []ToolDescriptor) string {
	encoded, _ := json.Marshal(tools)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

// FindTool is the tool of a name in a set.
func FindTool(tools []ToolDescriptor, name string) (ToolDescriptor, bool) {
	for _, tool := range tools {
		if tool.Name == name {
			return tool, true
		}
	}
	return ToolDescriptor{}, false
}
