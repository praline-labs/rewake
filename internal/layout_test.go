// Package internal holds the test of the layout of rewake 2.0: the import rule
// and the rule against a registry of adapters (docs/v2/design.md#the-test-that-holds-it,
// docs/rules/layout.md), and the callers of the mailbox's locked entry points.
// It judges the packages under internal/ and cmd/; test/ and tools/ are drivers
// outside the layers, and so is this package.
package internal

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

const (
	module     = "github.com/praline-labs/rewake"
	moduleRoot = ".."
)

// buildTags are the tags production code exists under. The rules run over
// every combination of them, so an edge or a registry that only a tagged build
// compiles is judged like any other.
var buildTags = []string{"rewakefault", "rewakefixture"}

// adapterInterfaces are the contract an adapter implements; the registry rule
// looks for variables holding values of it.
var adapterInterfaces = []string{"internal/adapter.Harness"}

func TestImportsFollowTheLayers(t *testing.T) {
	for _, problem := range importProblems(t) {
		t.Error(problem)
	}
}

func TestNoAdapterIsRegistered(t *testing.T) {
	for _, r := range registrations(t) {
		t.Error(r)
	}
}

// The rules run over whatever module lies at moduleRoot, so the cases that
// prove them can put a synthetic module there and go through the same
// discovery as the real run.

func importProblems(t *testing.T) []string {
	t.Helper()
	found, problems := judgeImports(variants(t))
	for _, e := range found {
		problems = append(problems, e.String())
	}
	return problems
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
	for _, dir := range packageDirs(t) {
		layer, ok := layerOfPath(dir)
		if !ok || !underAdapter(layer) {
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

// builds are what the rules read the module under: every combination of the
// production tags, each plain and with -race, which go test compiles with and
// which selects files of its own (//go:build race).
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
