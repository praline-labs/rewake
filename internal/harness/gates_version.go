package harness

import (
	"regexp"
	"time"
)

// A gate closed by a check is closed for the harness versions it ran
// against, so a launch reads its harness's version whenever the table holds
// such a version for that harness. A version that cannot be read leaves
// every gate open: the rules' action for what a gate leaves unknown is the
// safe one, never a closed gate taken on trust.

// versionBound bounds one --version: a cold start of a packaged harness
// answers well within it.
const versionBound = 5 * time.Second

// versionToken is a version in whatever a harness prints for --version:
// Codex answers "codex-cli 0.159.0", Claude Code "2.1.280 (Claude Code)".
var versionToken = regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?`)

// GatesFor is the gates of one launch: the version of the program it will
// run, read only when some gate is recorded closed for that harness, and the
// gates the caller takes as closed.
func GatesFor(harnessID, program string, env []string, dir string, assumed []string) Gates {
	version := ""
	if GatesNeedVersion(harnessID) {
		version = ReadVersion(program, env, dir)
	}
	return ResolveGates(harnessID, version, assumed)
}

// ReadVersion runs a program's --version as a check runs, bounded and
// reaped, and answers the first version it prints; "" when it printed none,
// exited otherwise than with 0, or did not answer in time.
func ReadVersion(program string, env []string, dir string) string {
	check, err := StartCheck(CheckSpec{Label: "--version", Program: program, Args: []string{"--version"}, Env: env, Dir: dir, Bound: versionBound, Capture: true})
	if err != nil {
		return ""
	}
	code, answered := check.Wait()
	if check.End() != nil || !answered || code != 0 {
		return ""
	}
	return versionToken.FindString(string(check.Output()))
}
