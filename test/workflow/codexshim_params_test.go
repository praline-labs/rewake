package workflow

import (
	"bytes"
	"encoding/json"
)

// What the shim accepts from a client, checked field by field.
//
// The shape of a request is checked explicitly, not by whether Go's decoder
// complained. It accepts JSON null into almost anything — a null object
// becomes an empty map, a null array element becomes an empty string — so
// "it parsed" says very little about what arrived.

// threadOf reads the conversation id a request names, if any.
func threadOf(params json.RawMessage) string {
	var asked struct {
		ThreadID string `json:"threadId"`
	}
	if json.Unmarshal(params, &asked) != nil {
		return ""
	}
	return asked.ThreadID
}

// usesExperimental reports whether a request carries a field that only an
// experimental client may use.
func usesExperimental(params json.RawMessage) bool {
	return hasField(params, "runtimeWorkspaceRoots")
}

// jsonKind reports what a raw JSON value is, or "" when it is unreadable.
// "" is a kind of its own here rather than a missing answer: it equals none of
// the kinds a description asks for, so an unreadable value is refused wherever
// a kind is compared.
func jsonKind(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || !json.Valid(trimmed) {
		return ""
	}
	switch trimmed[0] {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	default:
		return "number"
	}
}

// isObject reports whether params is a JSON object — null is not one.
func isObject(params json.RawMessage) bool { return jsonKind(params) == "object" }

// malformedField names the first field whose shape is wrong for the methods
// this shim serves.
func malformedField(params json.RawMessage) string {
	var fields map[string]json.RawMessage
	if json.Unmarshal(params, &fields) != nil {
		return "params"
	}
	if raw, ok := fields["config"]; ok && jsonKind(raw) != "object" && jsonKind(raw) != "null" {
		return "config must be an object"
	}
	if raw, ok := fields["threadSource"]; ok && jsonKind(raw) != "string" {
		return "threadSource must be a string"
	}
	if raw, ok := fields["threadId"]; ok && jsonKind(raw) != "string" {
		return "threadId must be a string"
	}
	if raw, ok := fields["runtimeWorkspaceRoots"]; ok && jsonKind(raw) != "null" {
		if jsonKind(raw) != "array" {
			return "runtimeWorkspaceRoots must be a list of paths"
		}
		var roots []json.RawMessage
		if json.Unmarshal(raw, &roots) != nil {
			return "runtimeWorkspaceRoots must be a list of paths"
		}
		for _, root := range roots {
			if jsonKind(root) != "string" {
				return "every runtimeWorkspaceRoots entry must be a path"
			}
		}
	}
	return ""
}

// rawField returns a field as it arrived, and whether it was there at all.
// The two answers have to stay separate: a decoder folds a missing field and a
// null one into the same zero value, and that is how a fixture ends up
// accepting requests the real server refuses.
func rawField(params json.RawMessage, name string) (json.RawMessage, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(params, &fields) != nil {
		return nil, false
	}
	raw, ok := fields[name]
	return raw, ok
}

// describeKind names what arrived, for a refusal that says what was wrong.
func describeKind(raw json.RawMessage) string {
	if kind := jsonKind(raw); kind != "" {
		return kind
	}
	return "something unreadable"
}

// hasField reports whether a JSON object carries a key at all.
func hasField(params json.RawMessage, name string) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(params, &fields) != nil {
		return false
	}
	_, ok := fields[name]
	return ok
}
