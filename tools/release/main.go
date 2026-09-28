// Command release is the gate every npm release of rewake goes through.
//
// Publishing is irreversible: a version, once taken in the registry, can never
// be published again, even after an unpublish, and a package that shipped a
// stray file stays in every mirror. So the gate does everything except publish
// unless told otherwise: it checks the repository and the registry, builds the
// packages with scripts/pack.sh, checks what each would upload and runs the
// binary and the shim built for this machine. Only --publish reaches
// npm publish, and only after every check passed.
//
// Why a Go program and not a script, as tools/checksummary and
// tools/harnesscache are: the gate reads npm's JSON answers — the files a
// package would upload, the error code of a registry lookup — and POSIX sh has
// no JSON reader short of a dependency the machine may lack. In Go the
// decisions are plain functions that the five checks test with a fake npm,
// which is the only way to test a publish without publishing.
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
	// exitDone: every check passed, and with --publish every package was
	// published.
	exitDone = 0
	// exitFailed: a check failed or a publish did not complete; the output
	// says which and what was and was not published.
	exitFailed = 1
	// exitCall: the call was wrong.
	exitCall = 2
)

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	c, err := parseArgs(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "release: "+err.Error())
		_, _ = fmt.Fprintln(stderr, "full help: go run ./tools/release --help")
		return exitCall
	}
	if c.help {
		usage(stdout)
		return exitDone
	}
	g := &gate{call: c, run: execute, out: stdout, arch: thisArch()}
	return g.release(ctx)
}

func usage(to io.Writer) {
	_, _ = fmt.Fprint(to, `release — build, check and, only when told, publish the npm packages of rewake

  go run ./tools/release <version>                        check everything, publish nothing
  go run ./tools/release <version> --publish [--otp <code>]

Without --publish nothing is uploaded: the gate checks, builds and reports, and
a dry run keeps going past a failed check so that one run shows every problem.
With --publish a failed check before the build stops the release there, and
nothing is published unless every check passed.

The checks, in order:
  - the working tree is clean and origin/main contains HEAD;
  - internal/cli/registry.go at HEAD says var Version = "<version>", which
    the release commit sets;
  - the version is semver without build metadata, which npm would strip,
    and none of the three packages has it in the
    registry: only npm's own E404 counts as free, and any other answer —
    a network failure, a 403, an answer that does not parse — stops the
    release as unsafe to decide;
  - scripts/pack.sh builds the packages into dist/npm;
  - each package would upload exactly its allowlist of files, nothing missing
    and nothing extra, and carries the right name, version, os and cpu;
  - each platform binary is built for its architecture, and the one for this
    machine prints the version, built from HEAD with no local changes;
  - the entry's shim, laid out in a node_modules with this machine's platform
    package, runs that binary, and without it fails naming what to install;
  - the shim's own tests, go test ./scripts.

--publish then uploads both platform packages first and the entry last, all
with --access public to https://registry.npmjs.org/, and stops at the first
failure, naming what was and was not published. It asks the registry for each
package afterwards and reports; a package not visible yet a minute after its
upload is reported and ends the run with 1 and no tag, since only a later
look tells a delay from an upload npm skipped. The git tag is printed, not
created.

  go run ./tools/release 1.0.0
  go run ./tools/release 1.0.0 --publish --otp 123456

Flags
  --publish       upload the packages once every check passed
  --otp <code>    a one-time password, passed to npm publish
  --help          this text

Exit codes
  0  every check passed; with --publish, every package was published
  1  a check failed, a publish did not complete, or a package it published
     is not visible yet
  2  the call was wrong
`)
}
