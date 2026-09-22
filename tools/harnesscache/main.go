// Command harnesscache fetches a harness version from the npm registry once,
// keeps it unpacked in a cache, and runs it in a disposable container.
//
// Why it exists: to learn what a new harness version breaks before the owner
// updates their own installation, never by testing on it. The versions are
// 100 MB and more, so each is downloaded once and reused; the workflow suite
// reads the same cache through the same package when REWAKE_CODEX_VERSION
// names a version.
//
// Why a Go program and not a script: the same reason as tools/checksummary.
// It is checked by the five checks, runs the same everywhere, and the suite
// imports its packages instead of parsing its output.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iiiokojiadbi/rewake/tools/harnesscache/cache"
	"github.com/iiiokojiadbi/rewake/tools/harnesscache/container"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// Exit codes, printed in the usage text.
const (
	// exitDone: the command did what it was asked.
	exitDone = 0
	// exitFailed: the version could not be resolved, fetched, verified, run
	// or removed; the message says which and what to do.
	exitFailed = 1
	// exitCall: the call was wrong.
	exitCall = 2
)

// runTimeout bounds a harness run in the container. What is run today is a
// schema generation of a few seconds.
const runTimeout = 5 * time.Minute

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	call, err := parseArgs(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "harnesscache: "+err.Error())
		_, _ = fmt.Fprintln(stderr, "full help: go run ./tools/harnesscache --help")
		return exitCall
	}
	if call.command == "help" {
		usage(stdout)
		return exitDone
	}
	root := call.cache
	if root == "" {
		if root, err = cache.DefaultRoot(); err != nil {
			return failed(stderr, err)
		}
	}
	store := &cache.Store{Root: root, Registry: call.registry, Log: stderr}
	if store.Registry == "" {
		store.Registry = cache.DefaultRegistry
	}

	switch call.command {
	case "list":
		entries, err := store.List()
		if err != nil {
			return failed(stderr, err)
		}
		if call.json {
			return printJSON(stdout, stderr, entries)
		}
		if len(entries) == 0 {
			_, _ = fmt.Fprintf(stdout, "nothing cached under %s\n", root)
		}
		for _, entry := range entries {
			note := ""
			if entry.Partial {
				note = "  unfinished fetch, never used; remove it with `remove " + entry.Harness + " " + entry.Version + "`"
			}
			_, _ = fmt.Fprintf(stdout, "%-7s %-22s %7.1f MB  %s%s\n", entry.Harness, entry.Version, float64(entry.Bytes)/1e6, entry.Dir, note)
		}
		return exitDone
	case "remove":
		removed, err := store.Remove(call.harness, call.selector)
		if err != nil {
			return failed(stderr, err)
		}
		for _, dir := range removed {
			_, _ = fmt.Fprintf(stdout, "removed %s\n", dir)
		}
		return exitDone
	}

	version, err := store.Get(ctx, call.harness, call.selector)
	if err != nil {
		return failed(stderr, err)
	}
	if call.command == "fetch" {
		if call.json {
			return printJSON(stdout, stderr, version)
		}
		_, _ = fmt.Fprintln(stdout, version.Executable)
		return exitDone
	}

	// run
	ctx, stop := context.WithTimeout(ctx, runTimeout)
	defer stop()
	if built, err := container.Ensure(ctx); err != nil {
		return failed(stderr, err)
	} else if built {
		_, _ = fmt.Fprintf(stderr, "built the image %s\n", container.Image())
	}
	out, err := container.Run(ctx, container.Spec{
		Version: version.Dir, Executable: version.ExecutableRel, Args: call.rest,
		Writable: call.write, WritableMount: writeMount, Network: call.network,
	})
	_, _ = stdout.Write(out)
	if err != nil {
		return failed(stderr, err)
	}
	return exitDone
}

// writeMount is where `run --write` puts the writable directory.
const writeMount = "/out"

func failed(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintln(stderr, "harnesscache: "+err.Error())
	return exitFailed
}

func printJSON(stdout, stderr io.Writer, value any) int {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return failed(stderr, err)
	}
	_, _ = fmt.Fprintln(stdout, string(encoded))
	return exitDone
}

func usage(to io.Writer) {
	_, _ = fmt.Fprint(to, `harnesscache — fetch a harness version once, keep it, run it in a container

  harnesscache fetch  <harness> <selector> [--json]
  harnesscache run    <harness> <selector> [--write <dir>] [--network <mode>] -- <args>
  harnesscache list   [--json]
  harnesscache remove <harness> <version>

Harnesses: codex, claude. A selector is an exact version (0.155.1), "latest"
(asks the registry, even when that version is cached) or "installed" (asks the
harness on PATH for --version, then fetches that version like any other).

fetch prints the path of the executable. A cached version is used without the
network; a missing one is downloaded, checked against the registry's sha512
integrity, unpacked into a temporary directory and only then renamed into
place, so an interrupted download is never taken for a cached version. Nothing
is ever removed automatically: list shows what is there, including unfinished
fetches, and remove deletes one version.

run starts the version in a fresh container: the version mounted read-only,
a private HOME on tmpfs, no network unless --network names one, the caller's
user id, nothing of the owner's home mounted, removed on exit. --write mounts
one host directory read-write at /out.

  go run ./tools/harnesscache fetch codex 0.156.0
  go run ./tools/harnesscache run codex 0.156.0 --write /tmp/schema -- app-server generate-json-schema --experimental --out /out
  go run ./tools/harnesscache run claude latest -- --version

Flags
  --cache <dir>      the cache. Default: $`+cache.RootEnv+`, else <cache home>/rewake/harness
  --registry <url>   the npm registry. Default: `+cache.DefaultRegistry+`
  --json             fetch and list: the machine form on standard output
  --write <dir>      run: the one writable mount, at /out
  --network <mode>   run: docker's network mode. Default: none
  --help             this text

Exit codes
  0  done
  1  the version could not be resolved, fetched, verified, run or removed
  2  the call was wrong
`)
}
