// Package internal holds the test of the layout of rewake 2.0: the three rules of
// docs/v2/design.md#the-test-that-holds-it, held from stage 3's first step on
// the 1.x tree through a transition table and an exception table
// (docs/v2/stage3-steps.md#s1-the-tests-first). It judges the packages under
// internal/ and cmd/; test/ and tools/ are drivers outside the layers, and so is
// this package, which imports the catalog only to learn the harness names.
package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
	_ "github.com/praline-labs/rewake/internal/harness/catalog"
)

const (
	module     = "github.com/praline-labs/rewake"
	moduleRoot = ".."
)

// buildTags are the tags production code exists under. The import rule runs
// over the graph of every combination of them, so an edge that only a tagged
// build compiles is judged like any other.
var buildTags = []string{"rewakefault"}

// ownWords belong to one harness without being its id or title. The list grows
// when a review finds a word.
var ownWords = []string{"mcp__", "CLAUDE_", "app-server", ".claude", ".codex", ".agents"}

// adapterInterfaces are the contract an adapter implements; rule 3 looks for
// variables holding values of it.
var adapterInterfaces = []string{"internal/harness.Harness"}

func TestImportsFollowTheLayers(t *testing.T) {
	for _, problem := range importProblems(t, transition, importExceptions) {
		t.Error(problem)
	}
}

func TestNoHarnessIsNamedOutsideTheAdapters(t *testing.T) {
	var names []string
	for _, h := range harness.All() {
		names = append(names, h.ID(), h.Title())
	}
	for _, problem := range nameProblems(t, harnessWords(names, ownWords), nameExceptions) {
		t.Error(problem)
	}
}

func TestNoAdapterIsRegistered(t *testing.T) {
	for _, problem := range registryProblems(t, registryExceptions) {
		t.Error(problem)
	}
}

// TestTheTablesShrinkWithTheMoves fails on a transition entry whose package is
// at its 2.0 path or gone, and on an exception without a reason or a step of
// the build order, so both tables empty as stage 3 goes rather than outliving
// what they excuse.
func TestTheTablesShrinkWithTheMoves(t *testing.T) {
	steps := buildSteps(t)
	for _, problem := range staleTransitions(transition, packageDirs(t, nil), steps) {
		t.Error(problem)
	}
	for _, problem := range unexplained(steps) {
		t.Error(problem)
	}
}

// The three rules run over whatever module lies at moduleRoot, so the cases
// that prove them can put a synthetic module there and go through the same
// discovery as the real run.

func importProblems(t *testing.T, table map[string]transit, exceptions []importException) []string {
	t.Helper()
	found, problems := judgeImports(variants(t), table)
	for _, e := range found {
		if !slices.ContainsFunc(exceptions, func(x importException) bool { return x.excuses(e) }) {
			problems = append(problems, e.String())
		}
	}
	return append(problems, unmatchedImports(exceptions, found)...)
}

func nameProblems(t *testing.T, words []string, table map[nameException]excuse) []string {
	t.Helper()
	return excused(mentions(t, words), nameKey, table)
}

func mentions(t *testing.T, words []string) []mention {
	t.Helper()
	var found []mention
	for _, dir := range packageDirs(t, named) {
		for _, name := range goFiles(t, dir) {
			src, err := os.ReadFile(filepath.Join(moduleRoot, name))
			if err != nil {
				t.Fatal(err)
			}
			mentions, err := findMentions(name, src, words)
			if err != nil {
				t.Fatal(err)
			}
			found = append(found, mentions...)
		}
	}
	return found
}

func registryProblems(t *testing.T, table map[registryException]excuse) []string {
	t.Helper()
	return excused(registrations(t), registryKey, table)
}

// registrations reads the inits of every file under adapter, whatever its
// constraint, and the variables of every package each build type-checks, tests
// included. A declaration two variants compile is reported once, known by its
// byte offset in its file: two names of one var spec, or two inits on one
// line, are two registrations.
func registrations(t *testing.T) []registration {
	t.Helper()
	var found []registration
	add := func(r registration) {
		if !slices.ContainsFunc(found, func(o registration) bool { return o.file == r.file && o.offset == r.offset }) {
			found = append(found, r)
		}
	}
	for _, dir := range packageDirs(t, nil) {
		p, err := placeOf(dir, transition)
		if err != nil || !underAdapter(p.layer) {
			continue
		}
		for _, name := range goFiles(t, dir) {
			src, err := os.ReadFile(filepath.Join(moduleRoot, name))
			if err != nil {
				t.Fatal(err)
			}
			inits, err := findInits(name, src)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range inits {
				add(r)
			}
		}
	}
	for _, b := range builds() {
		for _, r := range adapterVars(t, b) {
			add(r)
		}
	}
	return found
}

// builds are what rules 1 and 3 read the module under: every combination of
// the production tags, each plain and with -race, which the five checks' go
// test compiles with and which selects files of its own (//go:build race).
func builds() []build {
	var all []build
	for _, race := range []bool{false, true} {
		for _, tags := range tagSets() {
			all = append(all, build{tags: tags, race: race})
		}
	}
	return all
}

func tagSets() [][]string {
	sets := [][]string{{}}
	for _, tag := range buildTags {
		for _, set := range sets {
			sets = append(sets, append(slices.Clone(set), tag))
		}
	}
	return sets
}

// buildSteps are the steps of the build order in docs/v2/stage3.md, the only
// steps a table may name.
func buildSteps(t *testing.T) []string {
	t.Helper()
	steps, err := readSteps()
	if err != nil {
		t.Fatal(err)
	}
	return steps
}

func readSteps() ([]string, error) {
	text, err := os.ReadFile(filepath.Join(moduleRoot, "docs", "v2", "stage3.md"))
	if err != nil {
		return nil, err
	}
	steps, err := buildOrder(string(text))
	if err != nil {
		return nil, fmt.Errorf("docs/v2/stage3.md: %w", err)
	}
	return steps, nil
}

func validStep(step string, steps []string) bool { return slices.Contains(steps, step) }

func stepProblem(what, step string) string {
	return fmt.Sprintf("%s names the step %q, which is no step of docs/v2/stage3.md#the-build-order", what, step)
}
