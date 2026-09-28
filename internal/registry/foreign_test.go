package registry

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/state"
)

func TestListingForeignJSONNeverDeletesIt(t *testing.T) {
	dir := stateDir(t, map[int]uint64{})
	for _, raw := range []string{`{"id":"task","text":"keep"}`, `{"name":"foreign","servicePid":123}`, `{"name":"foreign","serviceStart":12}`} {
		path := filepath.Join(state.SessionsPath(dir), "foreign.json")
		if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		sessions, err := List(dir)
		if err != nil || len(sessions) != 0 {
			t.Errorf("foreign JSON listed: %+v %v", sessions, err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != raw {
			t.Fatalf("foreign JSON removed or changed: %q %v", got, err)
		}
	}
}
