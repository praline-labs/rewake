package cli

import (
	"testing"

	"github.com/praline-labs/rewake/internal/core/session"
)

func TestStateCellsShowTheModelNameWhenThereIsOne(t *testing.T) {
	id, name := "some-model-id[1m]", "Some Model 5"
	snapshot := session.Unknown("e")
	snapshot.Model, snapshot.Fresh, snapshot.SettingsFresh = &id, true, true
	if got := stateCells(&snapshot)[1]; got != `"some-model-id[1m]"` {
		t.Fatalf("without a name the cell is %s", got)
	}
	snapshot.ModelName = &name
	if got := stateCells(&snapshot)[1]; got != `"Some Model 5"` {
		t.Fatalf("with a name the cell is %s", got)
	}
}
