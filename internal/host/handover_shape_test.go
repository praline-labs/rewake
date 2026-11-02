package host

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// handoverShapes pins each handover version to the shape of what it carries:
// an image adopts only its own version, and a switch of the room's build
// admits only a build that takes the same, so a changed Handover raises
// HandoverVersion.
var handoverShapes = map[int]string{
	1: "a15c1ca7e2c400bd78be32764d513547e0dc1d21693149227891f9faedaea9de",
}

func TestTheHandoverContractChangesWithItsShape(t *testing.T) {
	sum := sha256.Sum256([]byte(shape(reflect.TypeFor[Handover]())))
	digest := hex.EncodeToString(sum[:])
	if pinned := handoverShapes[HandoverVersion]; pinned != digest {
		t.Fatalf("Handover's shape is %s, and handover %d pins %q: a changed Handover raises HandoverVersion and pins its shape", digest, HandoverVersion, pinned)
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
