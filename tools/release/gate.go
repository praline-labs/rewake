package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	gitLimit   = 30 * time.Second
	buildLimit = 10 * time.Minute
)

// gate is one run of the release gate.
type gate struct {
	call
	run  runner
	out  io.Writer
	root string
	head string
	// arch is this machine's npm cpu, "" when no package serves it.
	arch     string
	failures []string
}

func (g *gate) say(format string, args ...any) {
	_, _ = fmt.Fprintf(g.out, format+"\n", args...)
}

func (g *gate) pass(format string, args ...any) {
	g.say("ok    "+format, args...)
}

func (g *gate) fail(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	g.failures = append(g.failures, message)
	g.say("FAIL  %s", message)
}

// dist is where scripts/pack.sh puts a package.
func (g *gate) dist(p pkg) string { return filepath.Join(g.root, "dist", "npm", p.dir) }

// release runs the gate and returns the exit code.
func (g *gate) release(ctx context.Context) int {
	mode := "dry run: checks only, nothing is published"
	if g.publish {
		mode = "publishing once every check passes"
	}
	g.say("release %s — %s", g.version, mode)

	if g.root == "" {
		r := g.run(ctx, gitLimit, ".", "git", "rev-parse", "--show-toplevel")
		if !r.ok() {
			g.say("FAIL  not inside the rewake checkout: %s", r.why())
			return exitFailed
		}
		g.root = strings.TrimSpace(r.stdout)
	}
	if _, err := os.Stat(filepath.Join(g.root, "scripts", "pack.sh")); err != nil {
		g.say("FAIL  %s has no scripts/pack.sh: run the gate from the rewake checkout", g.root)
		return exitFailed
	}

	g.say("")
	g.say("before the build:")
	g.checkRepository(ctx)
	g.checkSourceVersion(ctx)
	g.checkRegistry(ctx)
	if g.publish && len(g.failures) > 0 {
		return g.refuse("nothing was built or published")
	}

	g.say("")
	g.say("building with scripts/pack.sh %s:", g.version)
	if r := g.run(ctx, buildLimit, g.root, "sh", filepath.Join(g.root, "scripts", "pack.sh"), g.version); !r.ok() {
		g.fail("scripts/pack.sh failed: %s", r.why())
		return g.refuse("nothing was published")
	}
	g.pass("packages built in %s", filepath.Join(g.root, "dist", "npm"))

	g.say("")
	g.say("the packages:")
	for _, p := range packages {
		g.checkPackage(ctx, p)
	}

	g.say("")
	g.say("on this machine:")
	g.checkBinary(ctx)
	g.checkShim(ctx)

	if !g.publish {
		g.say("")
		if len(g.failures) > 0 {
			g.say("dry run: nothing was published. %d check(s) failed:", len(g.failures))
			for _, failure := range g.failures {
				g.say("  - %s", failure)
			}
			return exitFailed
		}
		g.say("dry run: nothing was published. Every check passed; with the owner's word, publish exactly this:")
		g.say("  go run ./tools/release %s --publish", g.version)
		g.suggestTag()
		return exitDone
	}
	if len(g.failures) > 0 {
		return g.refuse("nothing was published")
	}

	g.say("")
	if !g.publishAll(ctx) {
		return exitFailed
	}
	if !g.verify(ctx) {
		// Published as far as npm said, not seen: no tag yet, and not a
		// success the caller could take for a finished release.
		return exitFailed
	}
	g.suggestTag()
	return exitDone
}

// refuse ends a release that cannot go on, naming every failed check.
func (g *gate) refuse(what string) int {
	g.say("")
	g.say("stopped: %s. %d check(s) failed:", what, len(g.failures))
	for _, failure := range g.failures {
		g.say("  - %s", failure)
	}
	return exitFailed
}

// checkRepository requires a clean tree whose HEAD is on origin/main: a
// release built from anything else cannot be found again by its tag.
func (g *gate) checkRepository(ctx context.Context) {
	r := g.run(ctx, gitLimit, g.root, "git", "status", "--porcelain")
	switch {
	case !r.ok():
		g.fail("could not read the working tree: %s", r.why())
	case strings.TrimSpace(r.stdout) != "":
		lines := strings.Split(strings.TrimRight(r.stdout, "\n"), "\n")
		shown := lines[:min(len(lines), 5)]
		more := ""
		if len(lines) > len(shown) {
			more = fmt.Sprintf(" and %d more", len(lines)-len(shown))
		}
		g.fail("the working tree has changes: %s%s; commit or stash them", strings.Join(shown, "; "), more)
	default:
		g.pass("the working tree is clean")
	}

	r = g.run(ctx, gitLimit, g.root, "git", "rev-parse", "HEAD")
	if !r.ok() {
		g.fail("could not read HEAD: %s", r.why())
		return
	}
	g.head = strings.TrimSpace(r.stdout)
	// The remote-tracking ref, as git push leaves it: asking the remote
	// itself would need its credentials for a check the push already made.
	r = g.run(ctx, gitLimit, g.root, "git", "merge-base", "--is-ancestor", g.head, "origin/main")
	switch {
	case r.ok():
		g.pass("origin/main contains HEAD %s", short(g.head))
	case r.err == nil && r.code == 1:
		g.fail("HEAD %s is not on origin/main: push it first", short(g.head))
	default:
		g.fail("could not tell whether origin/main contains HEAD %s: %s", short(g.head), r.why())
	}
}

// versionFile holds the version a build from source reports; pack.sh
// overrides it for the packages, and the release commit sets it to match.
const versionFile = "internal/cli/registry.go"

// versionLine is the declaration in versionFile.
var versionLine = regexp.MustCompile(`(?m)^var Version = "([^"]*)"$`)

// checkSourceVersion requires the committed default version to be the one
// released: otherwise the tagged source builds a binary that names another
// version than the one on npm. It reads HEAD, not the working tree, since the
// tag goes on HEAD.
func (g *gate) checkSourceVersion(ctx context.Context) {
	r := g.run(ctx, gitLimit, g.root, "git", "show", "HEAD:"+versionFile)
	if !r.ok() {
		g.fail("could not read %s at HEAD: %s", versionFile, r.why())
		return
	}
	match := versionLine.FindStringSubmatch(r.stdout)
	switch {
	case match == nil:
		g.fail("%s at HEAD has no line var Version = \"...\"", versionFile)
	case match[1] != g.version:
		g.fail("%s at HEAD says Version = %q, not %q: set it to var Version = %q in the release commit, and push", versionFile, match[1], g.version, g.version)
	default:
		g.pass("%s at HEAD says Version = %q", versionFile, match[1])
	}
}

// suggestTag prints the tag the release should get. Creating and pushing it
// is left to the owner: the gate changes nothing in the repository.
func (g *gate) suggestTag() {
	if g.head == "" {
		return
	}
	g.say("")
	g.say("tag the release (not done here):")
	g.say("  git tag -a v%s -m \"rewake %s\" %s && git push origin v%s", g.version, g.version, short(g.head), g.version)
}

func short(revision string) string {
	if len(revision) > 12 {
		return revision[:12]
	}
	return revision
}
