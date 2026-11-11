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
	8: "6bfc511d78d208986e333b44e2558b0c2c7ae37847957f3547401dc37e947225",
	7: "7dc1ff177d5a9dbf0345990eb103a3f3baae8ff6e21bd5c8132667be9a0f4813",
	6: "b1f9a548a8cff918aeb6c9384e2341238ecb82e4cb0c2632d994929e1c9f3313",
	1: "a15c1ca7e2c400bd78be32764d513547e0dc1d21693149227891f9faedaea9de",
	2: "796297c6eb8b5400402eb92666cc885e0e20db78a0387bd3efa3b32d31fafa3d",
	3: "e7798bc2d499a8ddc1b7ad123668267e34ba361b89e09ce13205d026fe457715",
	4: "274bf26cc9e0d69010003232189c905abcb352e576528a3058f31e0d76f7a39b",
	5: "310d27caa3d43f3ce1690766df78bf9c8a970f9fb7fb3a60b9c421fad95fc04f",
}

func TestTheHandoverContractChangesWithItsShape(t *testing.T) {
	sum := sha256.Sum256([]byte(shape(reflect.TypeFor[Handover]())))
	digest := hex.EncodeToString(sum[:])
	if pinned := handoverShapes[HandoverVersion]; pinned != digest {
		t.Fatalf("Handover's shape is %s, and handover %d pins %q: a changed Handover raises HandoverVersion and pins its shape", digest, HandoverVersion, pinned)
	}
}

// shape is a type as JSON carries it: each exported field's name, shape and
// tag, nested types included. A struct met again inside itself, as a message
// quoting a message, is its name: its shape is already written above.
func shape(t reflect.Type) string {
	return shapeIn(t, map[reflect.Type]bool{})
}

func shapeIn(t reflect.Type, open map[reflect.Type]bool) string {
	switch t.Kind() {
	case reflect.Pointer:
		return "*" + shapeIn(t.Elem(), open)
	case reflect.Slice:
		return "[]" + shapeIn(t.Elem(), open)
	case reflect.Map:
		return "map[" + shapeIn(t.Key(), open) + "]" + shapeIn(t.Elem(), open)
	case reflect.Struct:
		if open[t] {
			return t.String()
		}
		open[t] = true
		defer delete(open, t)
		var fields []string
		for index := range t.NumField() {
			if field := t.Field(index); field.IsExported() {
				fields = append(fields, fmt.Sprintf("%s %s %q", field.Name, shapeIn(field.Type, open), field.Tag.Get("json")))
			}
		}
		return "{" + strings.Join(fields, "; ") + "}"
	}
	return t.Kind().String()
}
