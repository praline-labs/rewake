package catalog

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/harness"
)

// A registered harness has to say which of its flags it takes at most once.
//
// The compiler makes the method impossible to forget, but the cheapest way to
// satisfy it is to return nothing — and nothing does not mean "no opinion", it
// means "every flag may be repeated". On a harness that refuses repeats that
// turns an alias plus a typed flag into a launch that will not parse, which is
// exactly the defect this list exists to prevent. So the list being non-empty
// is checked here rather than left to whoever adds the next harness.
func TestEveryHarnessNamesItsSingleUseFlags(t *testing.T) {
	for _, h := range harness.All() {
		t.Run(h.ID(), func(t *testing.T) {
			flags := h.SingleUseFlags()
			if len(flags) == 0 {
				t.Fatal("no single-use flags: an empty list says every flag may be repeated")
			}
			seen := map[string]bool{}
			for _, flag := range flags {
				if len(flag.Spellings) == 0 {
					t.Fatal("a flag with no spellings")
				}
				for _, spelling := range flag.Spellings {
					if !strings.HasPrefix(spelling, "-") {
						t.Fatalf("%q is not a flag", spelling)
					}
					if seen[spelling] {
						t.Fatalf("%q is listed twice; one spelling belongs to one parameter", spelling)
					}
					seen[spelling] = true
				}
			}
		})
	}
}

// A registered harness has to name where it keeps its own configuration. An
// empty list is the cheapest way to satisfy the method, and it would let main
// grant a worker the directory that decides what that harness runs.
func TestEveryHarnessNamesItsProtectedDirs(t *testing.T) {
	for _, h := range harness.All() {
		t.Run(h.ID(), func(t *testing.T) {
			dirs := h.ProtectedDirs()
			if len(dirs) == 0 {
				t.Fatal("no protected directories: a grant could reach this harness's own configuration")
			}
			for _, dir := range dirs {
				if !filepath.IsAbs(dir) {
					t.Fatalf("%q is not absolute: the hard tier drops it", dir)
				}
			}
		})
	}
}
