package harness

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Every adapter has a minimum version of its harness, and a launch reads the
// installed one before the run is claimed: a version below the minimum, or
// one that cannot be read, refuses the launch, so a refusal leaves nothing
// behind (docs/v2/design-api.md#minimum-versions). The read runs the program
// the launch will, as a probe: bounded, its output never shown.

// LaunchVersionReader is a harness whose launch reads its version before the
// claim, and refuses below its minimum.
type LaunchVersionReader interface {
	ReadLaunchVersion(program string, env []string, dir string) (Version, error)
}

// versionBound bounds one --version: a cold start of a packaged harness
// answers well within it; a variable so a test of a hanging one need not
// wait the whole of it.
var versionBound = 5 * time.Second

// versionToken is a version in whatever a harness prints for --version:
// Claude Code answers "2.1.287 (Claude Code)", the fixture "fixture 1.0.0".
var versionToken = regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?`)

// Version is what a launch read of its harness's version: the version, or
// how the read went when there is none.
type Version struct {
	Value string
	// Unknown, when Value is empty, says how the version was asked and
	// what came of it, in the words a diagnostic may show.
	Unknown string
}

// Note is the launch note's words for an unknown version, "" for a known one.
func (v Version) Note() string {
	if v.Value != "" {
		return ""
	}
	cause := v.Unknown
	if cause == "" {
		cause = "not read"
	}
	return "harness version unknown (" + cause + ")"
}

// ReadVersion runs a program's --version as a probe, bounded and reaped. It
// answers the version it printed, or an unknown version saying what came
// of the read — no start, no answer within the bound, an exit other than 0,
// no version in the output — once the holder has shown that everything the
// probe began has ended. When that cannot be shown it answers an error,
// which refuses the launch: it would otherwise run beside what is left.
func ReadVersion(program string, env []string, dir string) (Version, error) {
	asked := filepath.Base(program) + " --version"
	notEnded := fmt.Errorf("%s %s: what it started could not be shown to have ended", asked, OutcomeNotEnded)
	probe, err := StartProbe(ProbeSpec{Label: "--version", Program: program, Args: []string{"--version"}, Env: env, Dir: dir, Bound: versionBound, Capture: true})
	switch {
	case errors.Is(err, ErrProbeNotEnded):
		return Version{}, notEnded
	case err != nil:
		return Version{Unknown: asked + " " + OutcomeNoStart}, nil
	}
	code, answered := probe.Wait()
	if probe.End() != nil {
		return Version{}, notEnded
	}
	value := versionToken.FindString(string(probe.Output()))
	switch {
	case !answered:
		return Version{Unknown: asked + " gave " + OutcomeBound}, nil
	case code != 0:
		return Version{Unknown: asked + " ended with " + OutcomeExit(code)}, nil
	case value == "":
		return Version{Unknown: asked + " printed no version"}, nil
	}
	return Version{Value: value}, nil
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
