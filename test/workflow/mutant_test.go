package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
// the binary, or why it could not.
//
// It belongs to a case that has already started, and that is the point of its
// shape. Built before the case, a failed mutant published no record at all:
// the run went red, the summary could only say that no case explained it, and
// the build directory — the one piece of evidence — was deleted by t.TempDir
// on the way out. Now the directory belongs to the case, so it is kept when
// the case is red, and the error comes back to be recorded against the
// control's own observation.
func buildMutant(c *Case, m mutation) (string, error) {
	dir, err := os.MkdirTemp("", "rewake-mutant-"+m.name+"-")
	if err != nil {
		return "", fmt.Errorf("%s: a directory to build in: %w", m, err)
	}
	c.RemoveOnFinish(dir)
	fail := func(format string, args ...any) (string, error) {
		err := fmt.Errorf("%s: "+format, append([]any{m}, args...)...)
		// The reason goes beside the build, so the directory the verdict
		// points at says what happened in it.
		_ = os.WriteFile(filepath.Join(dir, "failure.txt"), []byte(err.Error()+"\n"), 0o600)
		return "", err
	}
	root, err := moduleRoot()
	if err != nil {
		return fail("%v", err)
	}
	original := filepath.Join(root, m.file)
	source, err := os.ReadFile(original)
	if err != nil {
		return fail("reading %s: %v", m.file, err)
	}
	text := string(source)
	for _, e := range m.edits {
		if n := strings.Count(text, e.old); n != 1 {
			return fail("the text to change occurs %d times in %s, not once; the product has moved and the control no longer breaks what it says", n, m.file)
		}
		text = strings.Replace(text, e.old, e.new, 1)
	}
	mutated := filepath.Join(dir, filepath.Base(m.file))
	if err := os.WriteFile(mutated, []byte(text), 0o600); err != nil {
		return fail("%v", err)
	}
	overlay, err := json.Marshal(map[string]any{"Replace": map[string]string{original: mutated}})
	if err != nil {
		return fail("%v", err)
	}
	overlayFile := filepath.Join(dir, "overlay.json")
	if err := os.WriteFile(overlayFile, overlay, 0o600); err != nil {
		return fail("%v", err)
	}
	binary := filepath.Join(dir, "rewake-"+m.name)
	ctx, stop := context.WithTimeout(context.Background(), buildTimeout)
	defer stop()
	// rewake reads no VCS stamp, and a checkout copied without its .git, as a
	// reviewer's snapshot is, fails the stamp rather than the mutant.
	build := exec.Command("go", "build", "-buildvcs=false", "-overlay", overlayFile, "-o", binary, "./cmd/rewake")
	build.Dir = root
	build.Env = os.Environ()
	if out, err := outputBounded(ctx, "go build", build); err != nil {
		_ = os.WriteFile(filepath.Join(dir, "build.log"), out, 0o600)
		return fail("building: %v", err)
	}
	return binary, nil
}

func (m mutation) String() string { return fmt.Sprintf("mutant %s (%s)", m.name, m.file) }
