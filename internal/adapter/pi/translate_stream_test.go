package pi

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mirror"
)

// streamed runs pi's events, as forward.ts gives them, through the shipped
// stream.ts and its link, and answers each batch it posted and the most it
// held unsent against its bound.
func streamed(t *testing.T, pushes string) (batches [][]json.RawMessage, most, held int) {
	t.Helper()
	bun, err := exec.LookPath("bun")
	if err != nil {
		t.Skip("bun is not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	script := `const { Stream, bounds } = await import("./stream.ts");
const batches = [];
let most = 0;
const stream = new Stream({ ready: () => true, post: async (lines) => { batches.push(lines); return true; }, after: () => {} });
stream.on = true;
const push = (event, delta, max) => { stream.push(event, delta, max); most = Math.max(most, stream.bytes); };
` + pushes + `
while (stream.bytes > 0) await stream.send();
console.log(JSON.stringify({ batches, most, held: bounds.held }));`
	command := exec.CommandContext(ctx, bun, "-e", script)
	command.Env = withoutRewakeEnv()
	out, err := command.Output()
	if err != nil {
		t.Fatalf("the stream: %v", err)
	}
	var got struct {
		Batches [][]json.RawMessage `json:"batches"`
		Most    int                 `json:"most"`
		Held    int                 `json:"held"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("the stream's batches: %v", err)
	}
	return got.Batches, got.Most, got.Held
}

// translated is what the translator made of the batches, one take each.
func translated(t *testing.T, batches [][]json.RawMessage) []mirror.Event {
	t.Helper()
	p := translating(t)
	for _, batch := range batches {
		lines := make([]string, len(batch))
		for i, line := range batch {
			lines[i] = string(line)
		}
		p.emit(lines...)
	}
	return p.out
}

// Review of b832ce0: the stream drops a long reply's every delta past its
// bound, and pi's aborted end, which says the reply whole, gives it to the
// face.
func TestAnAbortedReplyWhoseDeltasTheStreamDroppedReachesTheFaceWhole(t *testing.T) {
	batches, _, _ := streamed(t, `push({ event: "agent_start" });
push({ event: "message_update", type: "text_start", contentIndex: 0 });
push({ event: "message_update", type: "text_delta", contentIndex: 0, delta: "a".repeat(90000) }, true);
push({ event: "message_end", role: "assistant", stopReason: "aborted", content: [{ type: "text", text: "a".repeat(180000) }] });
push({ event: "agent_settled", aborted: true });`)
	lost := false
	for _, batch := range batches {
		for _, line := range batch {
			lost = lost || strings.Contains(string(line), `"event":"lost"`)
		}
	}
	if !lost {
		t.Fatal("the stream dropped no delta: the case does not stand")
	}
	for _, e := range translated(t, batches) {
		if e.Kind == mirror.EventSettle && e.Text == strings.Repeat("a", 180000) {
			return
		}
	}
	t.Fatal("the aborted reply never reached the face")
}

// Review of b832ce0: a call of many short hunks no string cut shortens is
// held within the stream's bound and reaches the face with its hunks' head.
func TestACallOfManyHunksStaysWithinTheStreamsBoundAndReachesTheFace(t *testing.T) {
	batches, most, held := streamed(t, `const edits = Array.from({ length: 1500 }, (_, i) => ({ oldText: "a".repeat(256) + i, newText: "b".repeat(256) + i }));
push({ event: "agent_start" });
push({ event: "tool_execution_start", toolCallId: "edit1", toolName: "edit", args: { path: "big.ts", edits } }, false, bounds.part);`)
	if most > held {
		t.Fatalf("the stream held %d bytes against its bound of %d", most, held)
	}
	var said []string
	for _, e := range translated(t, batches) {
		if e.ID == "edit1" {
			said = append(said, string(e.Kind)+" "+e.Name+" "+e.Text)
		}
	}
	if len(said) != 2 || !strings.HasPrefix(said[0], "tool ") || !strings.Contains(said[1], `"file_path":"big.ts"`) {
		t.Fatalf("the call as the face heard it: %.300q", said)
	}
}
