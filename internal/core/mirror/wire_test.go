package mirror

import (
	"encoding/json"
	"reflect"
	"testing"
)

// A batch as a wire carries it decodes into the model's events: every field
// carried, a kind or a turn's state the model does not know left out, and a
// mode named in the model's words by the adapter's table alone.
func TestABatchDecodesIntoTheModelsEvents(t *testing.T) {
	var sent []WireEvent
	body := `[
		{"kind":"turn","turn":"t","state":"working"},
		{"kind":"result","id":"c","status":"refused","text":"no","permit":{"by":"hook","allowed":false,"reason":"r"}},
		{"kind":"settle","id":"b","turn":"t","block":"text","part":1,"of":2,"text":"lo"},
		{"kind":"prompt","id":"p","origin":"person","status":"queued","text":"go"},
		{"kind":"resync","lost":3},
		{"kind":"mode","mode":"harness-plan"},
		{"kind":"mode","mode":"someday"},
		{"kind":"guess","id":"x"},
		{"kind":"turn","turn":"t","state":"ended"},
		{"kind":"turn","turn":"t"}
	]`
	if err := json.Unmarshal([]byte(body), &sent); err != nil {
		t.Fatal(err)
	}
	reason := "r"
	want := []Event{
		{Kind: EventTurn, Turn: "t", State: TurnStateWorking},
		{Kind: EventResult, ID: "c", Status: ItemStatusRefused, Text: "no", Permit: &Permit{By: PermitByHook, Reason: &reason}},
		{Kind: EventSettle, ID: "b", Turn: "t", Block: ItemKindText, Part: 1, Of: 2, Text: "lo"},
		{Kind: EventPrompt, ID: "p", Origin: "person", Status: ItemStatusQueued, Text: "go"},
		{Kind: EventResync, Lost: 3},
		{Kind: EventMode, Mode: PermissionModePlan},
		{Kind: EventMode},
	}
	got := Decode(sent, map[string]PermissionMode{"harness-plan": PermissionModePlan})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded\n%+v\nwant\n%+v", got, want)
	}
	if modes := Decode(sent[5:6], nil); len(modes) != 1 || modes[0].Mode != "" {
		t.Fatalf("a harness with no modes: %+v", modes)
	}
}
