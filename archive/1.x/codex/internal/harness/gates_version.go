package harness

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// A gate closed by a check is closed for the harness versions it ran
// against, and so is a bound L5 calibrated, so a launch takes its harness's
// version where a closed gate could change its choice
// (docs/mail-bridge-version.md). No program is run only to learn it: Codex's
// is the transport's own --version read, Claude Code's the path its binary
// resolves to. A version that is not known leaves every gate open: the rules'
// action for what a gate leaves unknown is the safe one, never a closed gate
// taken on trust.

// versionBound bounds one --version: a cold start of a packaged harness
// answers well within it; a variable so a test of a hanging one need not
// wait the whole of it.
var versionBound = 5 * time.Second

// versionToken is a version in whatever a harness prints for --version:
// Codex answers "codex-cli 0.159.0", Claude Code "2.1.280 (Claude Code)".
var versionToken = regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?`)

// Version is what a launch knows of its harness's version: the version, or
// why there is none.
type Version struct {
	Value string
	// Unknown is the cause when Value is empty: VersionNotRead,
	// VersionNotOnPath or VersionNotInPath.
	Unknown string
}

// The causes of an unknown version, as the launch note names them.
const (
	VersionNotRead   = "not read"
	VersionNotOnPath = "no claude on PATH"
	VersionNotInPath = "path names no version"
)

// Note is the launch note's words for an unknown version, "" for a known one.
func (v Version) Note() string {
	if v.Value != "" {
		return ""
	}
	cause := v.Unknown
	if cause == "" {
		cause = VersionNotRead
	}
	return "harness version unknown (" + cause + ")"
}

// ReadVersion runs a program's --version as a check runs, bounded and
// reaped. It answers one of three: the version it printed; unknown, once the
// holder has shown that everything the check began has ended, for no answer,
// an exit other than 0, no version in the output or the bound passing; or,
// when that cannot be shown, the check's failure, which refuses the launch
// under rule 6 before the claim.
func ReadVersion(program string, env []string, dir string) (Version, error) {
	notEnded := &CheckFailedError{Program: program, Label: "--version", Outcome: OutcomeNotEnded}
	check, err := StartCheck(CheckSpec{Label: "--version", Program: program, Args: []string{"--version"}, Env: env, Dir: dir, Bound: versionBound, Capture: true})
	switch {
	case errors.Is(err, ErrCheckNotEnded):
		return Version{}, notEnded
	case err != nil:
		return Version{Unknown: VersionNotRead}, nil
	}
	code, answered := check.Wait()
	if check.End() != nil {
		return Version{}, notEnded
	}
	value := versionToken.FindString(string(check.Output()))
	if !answered || code != 0 || value == "" {
		return Version{Unknown: VersionNotRead}, nil
	}
	return Version{Value: value}, nil
}

// claudeVersioned is the native installer's place for a version's binary.
var claudeVersioned = regexp.MustCompile(`/claude/versions/(` + versionToken.String() + `)$`)

// ClaudeVersion is the version of the claude a launch's PATH finds, read
// from the path it resolves to without running it: the native installer
// keeps each version at …/claude/versions/<version>. A --command wrapper
// does not change it (the owner's decision of October 4, 2026): one version
// runs for all sessions, and a wrapper starting another is not supported.
func ClaudeVersion(env []string) Version {
	path := ""
	for _, kv := range env {
		if value, ok := strings.CutPrefix(kv, "PATH="); ok {
			path = value
		}
	}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			dir = "."
		}
		candidate := filepath.Join(dir, "claude")
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
			continue
		}
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			return Version{Unknown: VersionNotInPath}
		}
		if match := claudeVersioned.FindStringSubmatch(resolved); match != nil {
			return Version{Value: match[1]}
		}
		return Version{Unknown: VersionNotInPath}
	}
	return Version{Unknown: VersionNotOnPath}
}
