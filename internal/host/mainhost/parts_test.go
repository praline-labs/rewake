package mainhost

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/parts"
)

// A compiled shell's hello says its face's contracts, and the door reports
// that face as within the shell, so the plan judges the shell's restart by
// them (found in review); a shell running a face from files reports itself
// alone, and an older shell's hello says no contracts.
func TestTheShellsHelloGivesItsParts(t *testing.T) {
	for _, c := range []struct {
		hello string
		want  []parts.Part
	}{
		{
			`{"hello":"shell","v":1,"contracts":{"requires":{"host":"1","face":"5"},"face":{"speaks":{"face":"5"},"requires":{"mirror":"1"}}}}`,
			[]parts.Part{
				{Name: parts.ShellPart, Class: parts.Shell, File: "exe1", Requires: map[string]string{"host": "1", "face": "5"}},
				{Name: parts.Face, Class: parts.Shell, Within: parts.ShellPart, Speaks: map[string]string{"face": "5"}, Requires: map[string]string{"mirror": "1"}},
			},
		},
		{
			`{"hello":"shell","v":1,"contracts":{"requires":{"host":"1","face":"5"}}}`,
			[]parts.Part{{Name: parts.ShellPart, Class: parts.Shell, File: "exe1", Requires: map[string]string{"host": "1", "face": "5"}}},
		},
		{
			`{"hello":"shell","v":1}`,
			[]parts.Part{{Name: parts.ShellPart, Class: parts.Shell, File: "exe1"}},
		},
	} {
		var hello Hello
		if err := json.Unmarshal([]byte(c.hello), &hello); err != nil {
			t.Fatal(err)
		}
		p := &shellProc{digested: make(chan struct{}), file: "exe1"}
		close(p.digested)
		p.took(hello)
		if got := p.parts(); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n  %+v\nwant\n  %+v", c.hello, got, c.want)
		}
	}
}
