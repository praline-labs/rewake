package pi

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// Each of pi's tools as the face reads it: every form the kin makes is a row
// of the face's table of its vocabulary (face/src/call.cases.ts), where what
// the face draws of it is said; a listing is the shell's ls, as Claude Code
// lists a directory, so it folds as a listed directory.
func TestPisToolsAreNamedAndGivenAsRowsOfTheFacesVocabulary(t *testing.T) {
	bun, err := exec.LookPath("bun")
	if err != nil {
		t.Skip("bun is not in PATH")
	}
	table, err := filepath.Abs("../../../face/src/call.cases.ts")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, bun, "-e", `const { cases } = await import(process.argv[1]); console.log(JSON.stringify(cases.map((c) => ({ tool: c.tool, input: c.input }))))`, table)
	command.Env = withoutRewakeEnv()
	out, err := command.Output()
	if err != nil {
		t.Fatalf("the face's table: %v", err)
	}
	var rows []struct {
		Tool  string `json:"tool"`
		Input any    `json:"input"`
	}
	if err := json.Unmarshal(out, &rows); err != nil || len(rows) == 0 {
		t.Fatalf("the face's table: %v\n%s", err, out)
	}
	forms := [][2]string{
		{"bash", `{"command":"make","timeout":5}`},
		{"read", `{"path":"a.go","offset":2}`},
		{"write", `{"path":"a","content":"x"}`},
		{"edit", `{"path":"a","edits":[{"oldText":"x","newText":"y"}]}`},
		{"edit", `{"path":"a","edits":[{"oldText":"x","newText":"y"},{"oldText":"p","newText":"q"}]}`},
		// A call's input while it streams is whatever parsed so far.
		{"edit", ``},
		{"grep", `{"pattern":"x","path":"src","ignoreCase":true}`},
		{"find", `{"pattern":"*.ts","path":"src","limit":3}`},
		{"ls", `{"path":"src"}`},
		{"ls", `{"path":"my dir"}`},
		{"ls", `{}`},
		{"brief_tick", `{"n":1}`},
	}
	for _, form := range forms {
		var args json.RawMessage
		if form[1] != "" {
			args = json.RawMessage(form[1])
		}
		call := facedTool(form[0], args)
		text, err := json.Marshal(call.input)
		if err != nil {
			t.Fatal(err)
		}
		var input any
		if err := json.Unmarshal(text, &input); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, row := range rows {
			found = found || row.Tool == call.name && reflect.DeepEqual(row.Input, input)
		}
		if !found {
			t.Errorf("%s %s is %s %s, no row of the face's table", form[0], form[1], call.name, text)
		}
	}
}

// A result's text is its text parts, an image named as one.
func TestAResultIsDrawnAsItsText(t *testing.T) {
	if got := resultText(json.RawMessage(`{"content":[{"type":"text","text":"a"},{"type":"image"},{"type":"text","text":"b"}]}`)); got != "a\n[image]\nb" {
		t.Fatalf("the result: %q", got)
	}
}
