// Command land takes an accepted branch into v2 the way root AGENTS.md asks
// of a step's last commit: it merges, runs the checks, and only on green
// pushes v2 and installs the build the room runs.
//
// Main landed every accepted branch with one long command repeated a dozen
// times a session, reading its large output each time. This prints a few
// lines — what landed, each check's verdict, what was pushed — and writes the
// rest to a log. Go rather than a script for the same reason as
// tools/release: the steps are plain functions the tests run with a fake git,
// go and bun, which is the only way to test a push without pushing.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// Exit codes, printed in the usage text.
const (
	// exitDone: landed, checked green, pushed and installed; with
	// --no-push, the checks were green and nothing changed.
	exitDone = 0
	// exitFailed: the state refused — not on v2, changes in the tree, a
	// conflict — or a check failed, or the push or the install did; the
	// output says which and where v2 is.
	exitFailed = 1
	// exitCall: the call was wrong.
	exitCall = 2
)

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	c, err := parseArgs(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "land: "+err.Error())
		_, _ = fmt.Fprintln(stderr, "full help: go run ./tools/land --help")
		return exitCall
	}
	if c.help {
		usage(stdout)
		return exitDone
	}
	if c.install == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "land: no home directory to find the install in; name it with --install <dir>")
			return exitCall
		}
		c.install = defaultInstall(home)
	}
	l := &lander{call: c, run: execute, out: stdout, processes: processes}
	return l.land(ctx)
}

func usage(to io.Writer) {
	_, _ = fmt.Fprint(to, `land — merge an accepted branch into v2, check it, push it and install the build

  go run ./tools/land <branch-or-commit> [--subject "<merge subject>"]
  go run ./tools/land <branch-or-commit> --no-push

Run from the main checkout with v2 checked out and a clean tree. The steps:
  - v2 is fast-forwarded to the commit when it can be; otherwise a merge
    commit is made with the subject --subject gives and the Co-Authored-By
    trailers of AGENTS.md's "Commits". A conflict aborts the merge and leaves
    the tree as it was;
  - the checks of a step's last commit, from AGENTS.md, each run with
    REWAKE_SESSION, REWAKE_EPOCH, REWAKE_DIR and REWAKE_ROOM cleared:
      race      go test -race -shuffle=on -timeout 20m -p <n> ./...,
                -p 4, or -p 2 while another go test -race or a run over
                ./... is in the process list
      workflow  REWAKE_WORKFLOW=1 go test -count=1 -timeout 10m -v ./test/workflow/...
      shell     bun install --frozen-lockfile && bun run check in shell/,
                only when shell/ or face/ changed between the old v2 and
                the new
      face      the same in face/, only when face/ changed
  - on green: git push origin v2, then go build of cmd/rewake into
    <install>/rewake.next, renamed over <install>/rewake; the room runs it
    once the owner restarts it;
  - on red, or on an interrupt during the checks: v2 is reset to where it was — nothing was pushed — and the failing
    checks are named.

It prints what landed, each check's verdict and the pushed commit, and the
path of a log, under .scratch/land/, with every command's output.

  go run ./tools/land wt-3da691
  go run ./tools/land wt-3da691 --subject "Merge tools/land into v2"
  go run ./tools/land 1a2b3c4 --no-push

Flags
  --subject <s>     the merge commit's subject, needed when v2 cannot be
                    fast-forwarded
  --no-push         a dry run: merge and check, then reset v2 to where it
                    was; nothing is pushed or installed
  --install <dir>   where the build goes (default ~/.local/share/rewake2)
  --help            this text

Exit codes
  0  landed, green, pushed and installed; with --no-push, green
  1  refused by the state, a conflict, a red check, or a failed push or
     install; the output says where v2 is
  2  the call was wrong
`)
}
