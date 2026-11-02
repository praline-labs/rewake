package mainhost

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/host/stream"
)

// handoverShapes pins each version of main's host's handover to the shape
// of what it carries, the handover and the screen file: an image adopts
// only its own version, and a switch of the room's build admits only a
// build that takes the same, so a changed shape raises HandoverVersion.
var handoverShapes = map[int]string{
	1: "e5d0ef7e5f7959dbd966e8c3b4c3e554e1c018363bb07aef862c7bd7d7d4e8fe",
}

func TestTheMainHostHandoverContractChangesWithItsShape(t *testing.T) {
	if stream.ScreenVersion != HandoverVersion {
		t.Fatalf("the screen file is version %d under handover.mainhost %d: raise both together", stream.ScreenVersion, HandoverVersion)
	}
	sum := sha256.Sum256([]byte(shape(reflect.TypeFor[Handover]()) + shape(reflect.TypeFor[stream.Screen]())))
	digest := hex.EncodeToString(sum[:])
	if pinned := handoverShapes[HandoverVersion]; pinned != digest {
		t.Fatalf("main's host's handover's shape is %s, and version %d pins %q: a changed shape raises HandoverVersion and pins its own", digest, HandoverVersion, pinned)
	}
}

// shape is a type as JSON carries it: each exported field's name, shape and
// tag, nested types included.
func shape(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Pointer:
		return "*" + shape(t.Elem())
	case reflect.Slice:
		return "[]" + shape(t.Elem())
	case reflect.Map:
		return "map[" + shape(t.Key()) + "]" + shape(t.Elem())
	case reflect.Struct:
		var fields []string
		for index := range t.NumField() {
			if field := t.Field(index); field.IsExported() {
				fields = append(fields, fmt.Sprintf("%s %s %q", field.Name, shape(field.Type), field.Tag.Get("json")))
			}
		}
		return "{" + strings.Join(fields, "; ") + "}"
	}
	return t.Kind().String()
}
