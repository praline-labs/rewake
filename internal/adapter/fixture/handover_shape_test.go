//go:build rewaketest

package fixture

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// handedShapes pins each version of the fixture's handed state to its fields:
// a switch of the room's build admits only a build that takes the same
// handover.fixture, so a changed handedState raises handoverVersion.
var handedShapes = map[int]string{
	1: `Version int "version"; PID int "pid"; Start uint64 "start"; Thread string "thread"; Program string "program"; Serves []string "serves"; Next int64 "next"`,
}

func TestTheFixtureHandoverChangesWithItsShape(t *testing.T) {
	typ := reflect.TypeFor[handedState]()
	fields := make([]string, 0, typ.NumField())
	for index := range typ.NumField() {
		field := typ.Field(index)
		fields = append(fields, fmt.Sprintf("%s %s %q", field.Name, field.Type, field.Tag.Get("json")))
	}
	if got := strings.Join(fields, "; "); got != handedShapes[handoverVersion] {
		t.Fatalf("handedState is\n%s\nand handover.fixture %d pins\n%s\na changed handedState raises handoverVersion", got, handoverVersion, handedShapes[handoverVersion])
	}
}
