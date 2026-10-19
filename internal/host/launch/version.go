package launch

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
)

// Every adapter declares the oldest version of its harness it starts and how
// to ask the installed program for its version (adapter.VersionGate), and a
// launch reads the installed one before the run is claimed: a version below
// the minimum, or one that cannot be read, refuses the launch, so a refusal
// leaves nothing behind (docs/v2/design-api.md#minimum-versions). The read runs
// the program the launch will, as a probe: bounded, its output never shown.

// versionBound bounds one read of a version: a cold start of a packaged harness
// answers well within it; a variable so a test of a hanging one need not
// wait the whole of it.
var versionBound = 5 * time.Second

// versionToken is a version in whatever a harness prints when asked: a
// version number, with or without the harness's name around it.
var versionToken = regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?`)

// Gate reads the installed version of a harness before the claim, asking the
// program the launch will start, and refuses a launch below the gate's minimum
// or without a version it could read. It answers the version read; a gate
// without a minimum reads nothing.
func Gate(h adapter.Harness, program string, env []string, dir string) (string, error) {
	gate := h.Version()
	if gate.Minimum == "" {
		return "", nil
	}
	version := readVersion(program, gate.Args, overlay(env, gate.Env), dir)
	return version.Value, RequireVersion(h.Title(), version, gate.Minimum)
}

// GateShipped reads the versions of a harness's shipped copy before the claim:
// its runtime's first, held to the runtime's minimum, then the copy's own,
// held to the pinned version exactly — this build was checked with that one.
func GateShipped(h adapter.Harness, start adapter.Start, env []string, dir string) (string, error) {
	if start.Runtime.Minimum != "" {
		runtime := readVersion(start.Command, start.Runtime.Args, env, dir)
		if err := RequireVersion(start.RuntimeTitle, runtime, start.Runtime.Minimum); err != nil {
			return "", err
		}
	}
	gate := h.Version()
	version := readVersion(start.Command, append(append([]string{}, start.Args...), gate.Args...), overlay(env, gate.Env), dir)
	return version.Value, RequirePinned(h.Title(), version, gate.Pinned)
}

// RequirePinned is the refusal of a shipped copy by version: nil only when
// the version read is the pinned one.
func RequirePinned(title string, version Version, pinned string) error {
	if version.Value == "" {
		cause := version.Unknown
		if cause == "" {
			cause = "not read"
		}
		return fmt.Errorf("the version of the %s shipped with rewake could not be read (%s); reinstall rewake", title, cause)
	}
	if version.Value != pinned {
		return fmt.Errorf("%s %s was found where rewake's own copy should be; this rewake runs %s %s: reinstall rewake, or name another program with --command", title, version.Value, title, pinned)
	}
	return nil
}

// overlay is env with each of set in place of a variable of the same name.
func overlay(env, set []string) []string {
	if len(set) == 0 {
		return env
	}
	named := map[string]bool{}
	for _, entry := range set {
		name, _, _ := strings.Cut(entry, "=")
		named[name] = true
	}
	out := make([]string, 0, len(env)+len(set))
	for _, entry := range env {
		if name, _, _ := strings.Cut(entry, "="); !named[name] {
			out = append(out, entry)
		}
	}
	return append(out, set...)
}

// Version is what a launch read of its harness's version: the version, or
// how the read went when there is none.
type Version struct {
	Value string
	// Unknown, when Value is empty, says how the version was asked and
	// what came of it, in the words a diagnostic may show.
	Unknown string
}

// readVersion asks a program its version as a probe, bounded, its group
// killed. It answers the version it printed, or an unknown version saying
// what came of the read — no start, no answer within the bound, an exit other
// than 0, no version in the output.
func readVersion(program string, args, env []string, dir string) Version {
	asked := strings.Join(append([]string{filepath.Base(program)}, args...), " ")
	probe := runProbe(program, args, env, dir, versionBound)
	value := versionToken.FindString(string(probe.Output))
	switch {
	case !probe.Started:
		return Version{Unknown: asked + " " + outcomeNoStart}
	case !probe.Answered:
		return Version{Unknown: asked + " gave " + outcomeBound}
	case probe.Code != 0:
		return Version{Unknown: asked + " ended with " + outcomeExit(probe.Code)}
	case value == "":
		return Version{Unknown: asked + " printed no version"}
	}
	return Version{Value: value}
}

// RequireVersion is a launch's refusal by version: nil when the version is
// known and at least minimum. title names the harness as a person knows it.
func RequireVersion(title string, version Version, minimum string) error {
	if version.Value == "" {
		cause := version.Unknown
		if cause == "" {
			cause = "not read"
		}
		return fmt.Errorf("the version of %s could not be read (%s); rewake needs %s or later", title, cause, minimum)
	}
	if VersionBelow(version.Value, minimum) {
		return fmt.Errorf("%s %s is installed; rewake needs %s or later", title, version.Value, minimum)
	}
	return nil
}

// VersionBelow reports whether version a comes before b, by their numeric
// parts. A part that is not a number counts as zero, which only makes a
// version older.
func VersionBelow(a, b string) bool {
	left, right := strings.Split(a, "."), strings.Split(b, ".")
	for i := range max(len(left), len(right)) {
		x, y := versionPart(left, i), versionPart(right, i)
		if x != y {
			return x < y
		}
	}
	return false
}

func versionPart(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	digits := parts[i]
	if cut := strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '9' }); cut >= 0 {
		digits = digits[:cut]
	}
	n, err := strconv.Atoi(digits)
	if err != nil {
		return 0
	}
	return n
}
