package workflow

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestStubScenario is the stage-0 scenario: it carries no claim about rewake's
// behavior, only that the scaffolding around a scenario works end to end —
// a binary built inside the run, a private world, named observations, and a
// verdict published after cleanup was checked.
//
// It is the shape every later scenario copies, which is why it exercises the
// cycle rather than asserting something interesting.
func TestStubScenario(t *testing.T) {
	binary := enterScenario(t, "stub")

	c := Start(t, Spec{
		Name: "stub",
		Observations: []string{
			"the run built its own binary",
			"no rewake is reachable through the isolated PATH",
			"the binary works against the case's own state directory",
		},
		Deadline: 30 * time.Second,
	})
	iso := Isolate(t, c, binary)

	c.Note("binary built")
	if !strings.HasPrefix(binary, "/") {
		c.Contradicted("the run built its own binary", "binary path is not absolute: %s", binary)
	} else {
		c.Observed("the run built its own binary", binary)
	}

	// A scenario that silently fell back to an installed rewake would report
	// on somebody else's build, so the absence is observed, not assumed.
	c.Note("looking for a stray rewake on PATH")
	switch found, ok, err := iso.Lookup("rewake"); {
	case err != nil:
		// Not being able to look is not the same as having looked: the
		// observation stays unmade rather than quietly satisfied.
		c.Contradicted("no rewake is reachable through the isolated PATH", "could not check: %v", err)
	case ok:
		c.Contradicted("no rewake is reachable through the isolated PATH", "found %s", found)
	default:
		c.Observed("no rewake is reachable through the isolated PATH", "not on the case's PATH")
	}

	c.Note("running the built binary")
	out, err := iso.Output(iso.Command("list", "--json"))
	if err != nil {
		c.Contradicted("the binary works against the case's own state directory", "rewake list: %v", err)
		return
	}
	var listed struct {
		Directory string `json:"directory"`
	}
	if err := json.Unmarshal(out, &listed); err != nil {
		c.Contradicted("the binary works against the case's own state directory", "unreadable output %q: %v", out, err)
		return
	}
	if listed.Directory != iso.StateDir {
		c.Contradicted("the binary works against the case's own state directory",
			"rewake used %s, the case owns %s", listed.Directory, iso.StateDir)
		return
	}
	c.Observed("the binary works against the case's own state directory", listed.Directory)
}
