package workflow

// What the checker must not let past, and what the shim must not accept.
//
// These came from the acceptance round that found eight quiet passes in the
// checker and three in the shim's request parsing. They are kept because each
// is a way a check could report success it did not earn — the failure this
// package exists to make impossible.

import (
	"encoding/json"
	"testing"
)

func TestCheckerSchemaRefusesUnsupportedOrInvalid(t *testing.T) {
	tests := []struct {
		name, schema string
		value        any
	}{
		{"minimum", `{"type":"integer","minimum":0}`, float64(-1)},
		{"maximum", `{"type":"number","maximum":1}`, float64(2)},
		{"oneOf-overlap", `{"oneOf":[{"type":"number"},{"type":"integer"}]}`, float64(1)},
		{"oneOf-empty", `{"oneOf":[]}`, "anything"},
		{"allOf-sibling-type", `{"allOf":[{}],"type":"string"}`, float64(1)},
		{"enum-type-coercion", `{"type":"string","enum":["1"]}`, float64(1)},
		{"unsupported-in-additionalProperties", `{"type":"object","properties":{},"additionalProperties":{"unknownConstraint":true}}`, map[string]any{"extra": 1}},
		{"properties-absent-but-additional-forbidden", `{"type":"object","additionalProperties":false}`, map[string]any{"extra": 1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := &schemaBundle{definitions: map[string]json.RawMessage{"X": json.RawMessage(tc.schema)}}
			if problems := b.check("X", tc.value); len(problems) == 0 {
				t.Errorf("invalid or unsupported schema/value silently passed: %s / %#v", tc.schema, tc.value)
			}
		})
	}
}

func TestCheckerShimRejectsMalformedSupportedRequests(t *testing.T) {
	for _, params := range []string{`null`, `{"runtimeWorkspaceRoots":[null]}`, `{"config":42}`} {
		t.Run(params, func(t *testing.T) {
			s := &shimSession{thread: shimThread}
			p := &shimPeer{initialized: true, experimental: true}
			_, _, err := s.answer(p, "thread/start", json.RawMessage(params))
			if err == nil {
				t.Errorf("malformed thread/start accepted: %s", params)
			}
		})
	}
}
