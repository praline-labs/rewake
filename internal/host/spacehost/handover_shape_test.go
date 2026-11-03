package spacehost

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/host/stream"
)

// handoverShapes pins each version of the space host's handover to the
// shape of what it carries, the spec of the stage adopt and the screen file:
// an image adopts only its own version, and a switch of the room's build
// admits only a build that takes the same, so a changed shape raises
// HandoverVersion, or ScreenVersion for the screen's: the contract is both.
var handoverShapes = map[string]string{
	"1.1": "bde0c1106b5fa47d171c8a5f681fae7a4eedd19e89fdc41b737a182e1cabfce0",
}

func TestTheSpaceHandoverContractChangesWithItsShape(t *testing.T) {
	sum := sha256.Sum256([]byte(shape(reflect.TypeFor[Spec]()) + shape(reflect.TypeFor[stream.Screen]())))
	digest := hex.EncodeToString(sum[:])
	if pinned := handoverShapes[Contract()]; pinned != digest {
		t.Fatalf("the space host's handover's shape is %s, and contract %s pins %q: a changed shape raises HandoverVersion or ScreenVersion and pins its own", digest, Contract(), pinned)
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
