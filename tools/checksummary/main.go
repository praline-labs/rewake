// Command checksummary turns a workflow suite run into a handful of lines and
// one machine-readable file.
//
// Why it exists: a suite run is hundreds of lines of `go test -v`, and an
// agent reads all of them to find two things — how many scenarios ran, and
// which observation failed. That is the cost this removes.
//
// Why it lives in tools/ rather than cmd/: cmd/ is what the project ships, and
// design.md names cmd/rewake as the entry point. This is a development tool,
// installed for nobody, and a second binary under cmd/ would say otherwise.
//
// What it does not do: run the five checks from AGENTS.md. Each of those is
// already one command with one line of output, and each prints its findings in
// its own words — wrapping them would add a second place where the list of
// checks can be forgotten, and AGENTS.md stays the one authority on what they
// are. This summarizes the suite, which is where the hundreds of lines are.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// Exit codes, printed in the usage text because a caller scripting this needs
// them to mean something stable.
const (
	// exitGreen: every case passed, or was unsupported for a named capability.
	exitGreen = 0
	// exitRed: a case failed, the run produced no scenarios, or the engine
	// itself reported a failure.
	exitRed = 1
	// exitCall: the call was wrong — an unknown flag, or no command to run.
	exitCall = 2
	// exitUnreadable: the stream could not be read, or carried an event this
	// program could not parse. Never silently skipped: a summary assembled
	// from part of a run is a summary that lies about the rest.
	exitUnreadable = 3
)

func run(args []string, stdout, stderr *os.File) int {
	options, command, err := parseArgs(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "checksummary: "+err.Error())
		usage(stderr)
		return exitCall
	}
	if options.help {
		usage(stdout)
		return exitGreen
	}

	started := time.Now()
	stream, engineExit, err := openStream(options, command, stderr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "checksummary: "+err.Error())
		return exitUnreadable
	}
	summary, err := parseStream(stream)
	closeErr := stream.Close()
	if err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "checksummary: "+err.Error())
		return exitUnreadable
	}
	summary.Wall = time.Since(started).Round(100 * time.Millisecond).String()
	summary.EngineExit = engineExit()

	path, writeErr := writeSummary(options.into, summary)
	if writeErr != nil {
		_, _ = fmt.Fprintf(stderr, "checksummary: the summary file could not be written: %v\n", writeErr)
		return exitUnreadable
	}
	summary.render(stdout, path)
	if summary.green() {
		return exitGreen
	}
	return exitRed
}

type options struct {
	into  string
	stdin bool
	help  bool
}

// defaultInto is ignored by git on purpose: a run leaves a directory behind
// every time, and a working tree that fills with them is a working tree whose
// status output stops being read.
const defaultInto = ".rewake-checks"

func parseArgs(args []string) (options, []string, error) {
	opts := options{into: defaultInto}
	if len(args) == 0 {
		return opts, nil, errors.New("nothing to do")
	}
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch {
		case argument == "--":
			command := args[index+1:]
			if opts.stdin && len(command) > 0 {
				return opts, nil, errors.New("--stdin reads a stream and a command runs one; choose one")
			}
			return opts, command, nil
		case argument == "--help" || argument == "-h":
			opts.help = true
			return opts, nil, nil
		case argument == "--stdin":
			opts.stdin = true
		case argument == "--into":
			if index+1 >= len(args) {
				return opts, nil, errors.New("--into needs a directory")
			}
			index++
			opts.into = args[index]
		case strings.HasPrefix(argument, "--into="):
			opts.into = strings.TrimPrefix(argument, "--into=")
		case strings.HasPrefix(argument, "-"):
			// A silently ignored flag returns an answer the caller believes.
			return opts, nil, fmt.Errorf("unknown flag %q", argument)
		default:
			if opts.stdin {
				return opts, nil, errors.New("--stdin reads a stream and a command runs one; choose one")
			}
			return opts, args[index:], nil
		}
	}
	if !opts.stdin {
		return opts, nil, errors.New("no command to run; pass one after -- or read a stream with --stdin")
	}
	return opts, nil, nil
}

func usage(to *os.File) {
	_, _ = fmt.Fprint(to, `checksummary — summarize a workflow suite run

  checksummary [--into <dir>] -- go test -count=1 -json ./test/workflow/...
  checksummary [--into <dir>] --stdin < run.jsonl

Runs the command, or reads a `+"`go test -json`"+` stream, and prints a few lines:
the scenarios that ran, the cases by outcome, and the path to summary.json.
A failing case is named with its harness column, its failing observation and
the directory that holds its evidence.

There is deliberately no --json on standard output. The machine-readable form is
the file under --into, and a second copy on the console would be the transcript
this program exists to replace.

Flags
  --into <dir>   where the summary goes. Default: `+defaultInto+`/<time>/summary.json
  --stdin        read the stream from standard input instead of running a command
  --help         this text

Exit codes
  0  every case passed, or was unsupported for a named capability
  1  a case failed, no scenario ran, or the engine reported a failure
  2  the call was wrong
  3  the stream could not be read or carried an event this program cannot parse

Run through `+"`go run`"+` these collapse: it reports any non-zero exit as 1 and
prints the real one as "exit status N". Build the binary to keep them apart.
`)
}

// writeSummary puts the file under a directory named by the run's time, so a
// second run never overwrites the evidence of the first.
func writeSummary(into string, summary *summary) (string, error) {
	directory := filepath.Join(into, time.Now().Format("2006-01-02T15-04-05"))
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(directory, "summary.json")
	encoded, err := summary.encode()
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, encoded, 0o644)
}
