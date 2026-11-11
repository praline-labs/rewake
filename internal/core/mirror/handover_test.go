package mirror

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// The model the next image adopts is the one the last image held: a face
// draws the same conversation, and what comes after the exec goes on from
// it — a settle half put together, a streaming reply, a tool running, the
// turn's times — as it would have in one image.
func TestTheNextImageAdoptsTheModelAsItWas(t *testing.T) {
	model := New()
	model.Apply(
		Event{Kind: EventResync, Lost: 2},
		Event{Kind: EventPrompt, ID: "p1", Turn: "t1", Text: "look", Origin: "person"},
		Event{Kind: EventTurn, State: TurnStateWorking},
		Event{Kind: EventDelta, ID: "b1", Turn: "t1", Block: ItemKindText, Text: "half "},
		Event{Kind: EventSettle, ID: "b1", Turn: "t1", Block: ItemKindText, Text: "the whole ", Part: 0, Of: 2},
		Event{Kind: EventTool, ID: "c1", Turn: "t1", Name: "Read"},
		Event{Kind: EventInput, ID: "c1", Text: strings.Repeat("x", MaxFieldBytes+10)},
		Event{Kind: EventMode, Mode: PermissionModeDefault},
		Event{Kind: EventWorkMode, WorkMode: "review"},
	)
	raw, err := model.Hand()
	if err != nil {
		t.Fatal(err)
	}
	adopted, err := Adopted(raw)
	if err != nil {
		t.Fatal(err)
	}
	same := func(when string) {
		t.Helper()
		if got, want := drawn(adopted), drawn(model); got != want {
			t.Fatalf("%s, the adopted model draws\n%s\nand the one handed over\n%s", when, got, want)
		}
		if adopted.bytes != model.bytes || len(adopted.parts) != len(model.parts) {
			t.Fatalf("%s, the adopted model counts %d bytes and %d settles, the one handed over %d and %d", when, adopted.bytes, len(adopted.parts), model.bytes, len(model.parts))
		}
	}
	same("adopted")
	after := []Event{
		{Kind: EventWorkMode, WorkMode: "build"},
		{Kind: EventSettle, ID: "b2", Turn: "t1", Block: ItemKindText, Text: "one ", Part: 0, Of: 2},
		{Kind: EventSettle, ID: "b2", Turn: "t1", Block: ItemKindText, Text: "two", Part: 1, Of: 2},
		{Kind: EventResult, ID: "c1", Text: "read", Status: ItemStatusDone},
		{Kind: EventTurn, State: TurnStateIdle},
	}
	model.Apply(after...)
	adopted.Apply(after...)
	same("after the same events")
}

// drawn is the snapshot a face connecting now reads.
func drawn(m *Model) string {
	var b strings.Builder
	for _, line := range m.Snapshot() {
		b.Write(encode(line))
	}
	return b.String()
}

// handedShapes pins each version of the handed model to its shape: a switch
// of the room's build admits only a build that takes the same, so a changed
// shape raises HandedVersion.
var handedShapes = map[int]string{
	2: "585326bbf9e8a3ea7875a69a482d02ca72a89cb4c2a74689ef81db34788bd49e",
	1: "e353d31385d8df3d96785639919835efeb37809c31f26d1bcd5a7fa038ac4150",
}

func TestTheHandedModelChangesWithItsShape(t *testing.T) {
	sum := sha256.Sum256([]byte(shapeOf(reflect.TypeFor[handedModel]())))
	if digest := hex.EncodeToString(sum[:]); digest != handedShapes[HandedVersion] {
		t.Fatalf("the handed model's shape is %s, and version %d pins %q: a changed shape raises HandedVersion and pins its own", digest, HandedVersion, handedShapes[HandedVersion])
	}
}

func shapeOf(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Pointer:
		return "*" + shapeOf(t.Elem())
	case reflect.Slice:
		return "[]" + shapeOf(t.Elem())
	case reflect.Map:
		return "map[" + shapeOf(t.Key()) + "]" + shapeOf(t.Elem())
	case reflect.Struct:
		var fields []string
		for index := range t.NumField() {
			if field := t.Field(index); field.IsExported() {
				fields = append(fields, fmt.Sprintf("%s %s %q", field.Name, shapeOf(field.Type), field.Tag.Get("json")))
			}
		}
		return "{" + strings.Join(fields, "; ") + "}"
	}
	return t.Kind().String()
}
