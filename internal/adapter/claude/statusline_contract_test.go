package claude

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// One reading of an interval serves the person's layers and the worker
// profile: kept as written from a second up, none for a value Claude Code
// would not run, and the run's own then stands.
func TestTheRefreshIntervalIsKeptAsWrittenOrNone(t *testing.T) {
	for _, c := range []struct {
		name  string
		value any
		want  json.Number
	}{
		{"a whole number", json.Number("7"), "7"},
		{"a fraction above one", json.Number("1.5"), "1.5"},
		{"under a second", json.Number("0.5"), ""},
		{"zero", json.Number("0"), ""},
		{"text", "7", ""},
		{"missing", nil, ""},
	} {
		line := map[string]any{"refreshInterval": c.value}
		if got := refreshOf(line); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
		setting := statusLineSetting("/bin/rewake", refreshOf(line))
		wantEvery := any(c.want)
		if c.want == "" {
			wantEvery = statusLineRefresh
		}
		if setting["refreshInterval"] != wantEvery {
			t.Errorf("%s: the run's line has %v, want %v", c.name, setting["refreshInterval"], wantEvery)
		}
	}
}

// A layer that does not parse as the worker profile's file would not adds
// nothing, trailing text included.
func TestALayerThatDoesNotParseAddsNoStatusLine(t *testing.T) {
	dir := t.TempDir()
	good, bad := filepath.Join(dir, "good.json"), filepath.Join(dir, "bad.json")
	writeLayer(t, good, `{"statusLine":{"type":"command","command":"x"}}`)
	writeLayer(t, bad, `{"statusLine":{"type":"command","command":"x"}} trailing`)
	if statusLineIn(good) == nil || statusLineIn(bad) != nil || statusLineIn("") != nil {
		t.Fatalf("good %v, bad %v", statusLineIn(good), statusLineIn(bad))
	}
}
