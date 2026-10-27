package cli

import (
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/session"
)

// The shell reads a session's telemetry from `rewake list --json` by types it
// writes by hand (shell/src/room.ts): their names are held here to what this
// build emits, so a rename fails before the strip or the bar shows nothing.
func TestTheShellsTypesOfTelemetry(t *testing.T) {
	source, err := os.ReadFile("../../shell/src/room.ts")
	if err != nil {
		t.Fatal(err)
	}

	_, _, peer := stateCaller(t, "main")
	code, out, errOut := run("list", "--json")
	if code != ExitOK {
		t.Fatalf("list: %d %q", code, errOut)
	}
	var listing struct {
		Sessions []struct {
			Name      string                     `json:"name"`
			Telemetry map[string]json.RawMessage `json:"telemetry"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(out), &listing); err != nil {
		t.Fatal(err)
	}
	var verdict map[string]any
	for _, s := range listing.Sessions {
		if s.Name == peer.Name {
			if err := json.Unmarshal(s.Telemetry["verdict"], &verdict); err != nil {
				t.Fatalf("%s's telemetry has no verdict: %v", s.Name, err)
			}
		}
	}
	if verdict["label"] != "working" || verdict["mark"] != session.MarkSteady {
		t.Fatalf("verdict = %v", verdict)
	}

	fields, unions := tsType(t, string(source), "Verdict")
	if got, want := strings.Join(fields, ","), strings.Join(keys(t, verdict), ","); got != want {
		t.Errorf("the shell's Verdict has %s, rewake emits %s", got, want)
	}
	for field, want := range map[string][]string{
		"mark":    {session.MarkOff, session.MarkSteady, session.MarkBlink},
		"blocked": {"", session.BlockedPermission, session.BlockedQuestion, session.BlockedDialog},
	} {
		slices.Sort(want)
		if got := unions[field]; !slices.Equal(got, want) {
			t.Errorf("the shell's Verdict.%s is %q, rewake's values are %q", field, got, want)
		}
	}

	var emitted []string
	snapshot := reflect.TypeFor[session.Snapshot]()
	for i := range snapshot.NumField() {
		emitted = append(emitted, strings.Split(snapshot.Field(i).Tag.Get("json"), ",")[0])
	}
	fields, _ = tsType(t, string(source), "Telemetry")
	for _, field := range fields {
		if !slices.Contains(emitted, field) {
			t.Errorf("the shell's Telemetry reads %s, which rewake does not emit", field)
		}
	}
}

// tsType is the sorted field names of a TypeScript object type declared at
// the top of a source, and for each field typed as a union of string literals,
// those literals sorted.
func tsType(t *testing.T, source, name string) ([]string, map[string][]string) {
	t.Helper()
	body := regexp.MustCompile(`(?ms)^(?:export )?type ` + name + ` = \{\n(.*?)^\};`).FindStringSubmatch(source)
	if body == nil {
		t.Fatalf("no type %s in the shell's source", name)
	}
	var fields []string
	unions := map[string][]string{}
	for _, m := range regexp.MustCompile(`(?m)^  (\w+)\??: (.*);$`).FindAllStringSubmatch(body[1], -1) {
		fields = append(fields, m[1])
		var literals []string
		for _, alt := range strings.Split(m[2], " | ") {
			if len(alt) < 2 || alt[0] != '"' || alt[len(alt)-1] != '"' {
				literals = nil
				break
			}
			literals = append(literals, alt[1:len(alt)-1])
		}
		if literals != nil {
			slices.Sort(literals)
			unions[m[1]] = literals
		}
	}
	slices.Sort(fields)
	return fields, unions
}
