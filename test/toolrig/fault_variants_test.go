//go:build rewakefixture

package toolrig

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// The paths that hold a record, beyond a new operation: the same words
// joining one, a retry of one left open, and the same words from a later
// turn, rebuilt from bridge/server/fault_variants_test.go. Each is built from
// a scenario of a new operation.

func init() {
	pending := faultScenario{
		name: "a pending mark",
		prepare: func(r *rig) []string {
			r.letter(faultLetter)
			r.nextTurn()
			r.start()
			read := r.call("inbox")
			if read.result.IsError || r.complete(read, true) != nil {
				r.t.Fatalf("the task was not read: %+v", read.result)
			}
			return []string{"pending", "the owner's answer"}
		},
		acceptable: func(text string) bool {
			return strings.Contains(text, "receipt") && !strings.Contains(text, "exit status")
		},
		shows:   func(string) bool { return true },
		settled: func(r *rig) bool { return marks(r) > 0 },
		absent: func(text string) bool {
			return (strings.Contains(text, "not marked") || strings.Contains(text, "so it was not taken")) && !retryWords.MatchString(text)
		},
		check: func(t *testing.T, r *rig) {
			if n := marks(r); n > 1 {
				t.Fatalf("%d pending marks for one call", n)
			}
		},
	}
	faultScenarios = append(faultScenarios, pending)
	bases := append([]faultScenario(nil), faultScenarios...)
	for _, base := range bases {
		if base.name == "the next part of a read" {
			continue
		}
		faultScenarios = append(faultScenarios, joining(base), retrying(base), laterTurn(base))
	}
}

// joining runs the call once clean, and breaks the same words again.
func joining(base faultScenario) faultScenario {
	s := base
	s.name = base.name + ", the same words again"
	s.prepare = func(r *rig) []string {
		words := base.prepare(r)
		r.start()
		first := r.call(words...)
		_ = r.complete(first, !first.result.IsError)
		return words
	}
	s.acceptable = func(text string) bool { return base.acceptable(text) || strings.Contains(text, "ran earlier") }
	return s
}

// lastStep is how many durable steps a scenario's child takes, learned once
// from a clean logged run.
func lastStep(base faultScenario) func(t *testing.T) int {
	var once sync.Once
	steps := 0
	return func(t *testing.T) int {
		once.Do(func() {
			probe := newRig(t)
			words := base.prepare(probe)
			logPath := filepath.Join(probe.root, "probe.log")
			probe.fault = "child:log=" + logPath
			probe.start()
			probe.call(words...)
			steps = len(durable(parseLog(t, logPath, probe.root), "child"))
		})
		return steps
	}
}

// openOperation makes the scenario's call and ends its child before its last
// durable step, which leaves the operation open; it returns the token.
func openOperation(r *rig, base faultScenario, last func(*testing.T) int) ([]string, string) {
	words := base.prepare(r)
	r.fault = fmt.Sprintf("child:crash=%d", last(r.t))
	r.start()
	c := r.call(words...)
	_ = r.complete(c, false)
	r.fault = ""
	found := retryWords.FindStringSubmatch(c.result.text())
	if found == nil {
		r.t.Fatalf("no operation was left open: %q", c.result.text())
	}
	return words, found[1]
}

// retrying breaks the retry of an operation left open.
func retrying(base faultScenario) faultScenario {
	s := base
	last := lastStep(base)
	s.name = base.name + ", its retry"
	s.prepare = func(r *rig) []string {
		_, token := openOperation(r, base, last)
		return []string{"retry", token}
	}
	s.acceptable = func(text string) bool { return base.acceptable(text) || strings.Contains(text, "ran earlier") }
	return s
}

// laterTurn breaks the same words in the next turn, which stop at the
// operation an earlier turn left open.
func laterTurn(base faultScenario) faultScenario {
	s := base
	last := lastStep(base)
	s.name = base.name + ", the same words a turn later"
	s.prepare = func(r *rig) []string {
		words, _ := openOperation(r, base, last)
		r.endTurn()
		r.nextTurn()
		return words
	}
	s.acceptable = func(text string) bool { return base.acceptable(text) || retryWords.MatchString(text) }
	return s
}
