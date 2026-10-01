package cli

import (
	"fmt"
	"os"
	"testing"

	"github.com/praline-labs/rewake/internal/cutover"
	"github.com/praline-labs/rewake/internal/grant"
	"github.com/praline-labs/rewake/internal/proc"
)

// Every directory these tests grant lies in a temporary directory, which the
// hard tier refuses; that refusal has its own test in internal/grant.
//
// The launches these tests make look for earlier-build writers in an empty
// process tree: the look's own tests describe the trees it judges, and a
// launch here must not depend on what else runs on the machine.
func TestMain(m *testing.M) {
	grant.TempRoots = func() []string { return nil }
	tree, err := os.MkdirTemp("", "rewake-proc-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cutover.Tree = proc.Reader{Root: tree}
	code := m.Run()
	_ = os.RemoveAll(tree)
	os.Exit(code)
}
