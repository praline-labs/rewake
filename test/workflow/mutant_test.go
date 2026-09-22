package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A control that breaks a property of the product breaks the product, not the
// fixture: the binary a control runs against is rewake with one line changed.
// The change is applied through the Go toolchain's overlay, so the working
// tree is never touched and nothing in the product knows it can be mutated.
//
// This is what "mutate the product, not the fixture" means in practice. A
// control expressed in the shim proves that the scenario notices a broken
// peer; one expressed in the product proves that it notices a broken rewake,
// which is the only thing the suite is for.

// mutation is a change to one file of the product: one edit, or a few that
// together remove one property. Each old text must occur exactly once: an
// edit that matches nothing has mutated nothing, and a control built on it
// would be a control of the healthy product.
type mutation struct {
	name  string
	file  string
	edits []edit
}

type edit struct{ old, new string }

// buildMutant builds rewake with the mutation applied and answers the path of
// the binary. The build runs in the module root, like the ordinary one, with
// the mutated file laid over the original.
func buildMutant(t *testing.T, m mutation) string {
	t.Helper()
	root, err := moduleRoot()
	if err != nil {
		t.Fatalf("mutant %s: %v", m.name, err)
	}
	original := filepath.Join(root, m.file)
	source, err := os.ReadFile(original)
	if err != nil {
		t.Fatalf("mutant %s: reading %s: %v", m.name, m.file, err)
	}
	text := string(source)
	for _, e := range m.edits {
		if n := strings.Count(text, e.old); n != 1 {
			t.Fatalf("mutant %s: the text to change occurs %d times in %s, not once; the product has moved and the control no longer breaks what it says", m.name, n, m.file)
		}
		text = strings.Replace(text, e.old, e.new, 1)
	}
	dir := t.TempDir()
	mutated := filepath.Join(dir, filepath.Base(m.file))
	if err := os.WriteFile(mutated, []byte(text), 0o600); err != nil {
		t.Fatalf("mutant %s: %v", m.name, err)
	}
	overlay, err := json.Marshal(map[string]any{"Replace": map[string]string{original: mutated}})
	if err != nil {
		t.Fatalf("mutant %s: %v", m.name, err)
	}
	overlayFile := filepath.Join(dir, "overlay.json")
	if err := os.WriteFile(overlayFile, overlay, 0o600); err != nil {
		t.Fatalf("mutant %s: %v", m.name, err)
	}
	binary := filepath.Join(dir, "rewake-"+m.name)
	ctx, stop := context.WithTimeout(context.Background(), buildTimeout)
	defer stop()
	build := exec.Command("go", "build", "-overlay", overlayFile, "-o", binary, "./cmd/rewake")
	build.Dir = root
	build.Env = os.Environ()
	if out, err := outputBounded(ctx, "go build", build); err != nil {
		t.Fatalf("mutant %s: building: %v: %s", m.name, err, out)
	}
	return binary
}

func (m mutation) String() string { return fmt.Sprintf("mutant %s (%s)", m.name, m.file) }
