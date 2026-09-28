package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/praline-labs/rewake/test/workflow/record"
)

// Where the stream comes from, and what is read out of it.
//
// Two inputs, both giving the same thing: the lines `go test -json` prints. A
// command is the ordinary form, because only the process that ran the engine
// knows what the engine returned, and that number belongs in the summary. A
// stream on standard input is for composing with something else, and then the
// engine's own exit code is unknown rather than guessed.

// openStream answers the reader, a function that reports the engine's exit
// code once the stream has ended, and an error. The exit code is a function
// rather than a value because it is not known until the command has finished,
// which is after the last line has been read.
func openStream(opts options, command []string, stderr *os.File) (io.ReadCloser, func() *int, error) {
	if opts.stdin {
		return io.NopCloser(os.Stdin), func() *int { return nil }, nil
	}
	if len(command) == 0 {
		return nil, nil, errors.New("no command to run")
	}
	engine := exec.Command(command[0], command[1:]...)
	engine.Stderr = stderr
	out, err := engine.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := engine.Start(); err != nil {
		return nil, nil, fmt.Errorf("starting %s: %w", command[0], err)
	}
	code := -1
	stream := &engineStream{ReadCloser: out, engine: engine, code: &code}
	return stream, func() *int {
		if code < 0 {
			// The command has not finished, or ended in a way that has no exit
			// code at all. Unknown is its own answer here, not zero.
			return nil
		}
		value := code
		return &value
	}, nil
}

// engineStream closes by waiting for the command, so the exit code is known by
// the time the summary is assembled. A reader that closed the pipe and walked
// away would leave the summary guessing at the one number it was given the
// command to learn.
type engineStream struct {
	io.ReadCloser
	engine *exec.Cmd
	code   *int
}

func (s *engineStream) Close() error {
	_ = s.ReadCloser.Close()
	err := s.engine.Wait()
	var exit *exec.ExitError
	switch {
	case err == nil:
		*s.code = 0
	case errors.As(err, &exit):
		*s.code = exit.ExitCode()
	default:
		return err
	}
	return nil
}

// event is the part of a `go test -json` line this program reads. Fields it
// does not use are ignored on purpose: the engine adds them over time, and a
// summarizer that refused an unknown field would break on a Go release.
//
// What is not ignored is a line it cannot read at all. That is the difference
// between "this record says more than I need" and "I do not know what this
// run did".
type event struct {
	Action  string `json:"Action"`
	Test    string `json:"Test"`
	Package string `json:"Package"`
	Output  string `json:"Output"`
}

// parseStream reads the engine's output and builds the summary from the
// records the suite printed — never from the prose around them. An added
// print cannot grow the summary, because nothing but a marked record is read.
//
// Output events are not lines. The engine splits a long one across several of
// them, and a record of a case with a dozen observations is long: reading each
// event as a line cut the first real run's records in half. So the text is
// reassembled per test before anything is looked for in it.
func parseStream(from io.Reader) (*summary, error) {
	found := &summary{}
	partial := map[string]string{}
	scanner := bufio.NewScanner(from)
	// A single test line can be long: a failing case prints its verdict with
	// paths, and a record carries every observation of a case.
	scanner.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var current event
		if err := json.Unmarshal([]byte(line), &current); err != nil {
			// Not skipped: a run whose stream this program cannot read is a
			// run it cannot summarize, and a summary of part of a run says
			// nothing about the rest.
			return nil, fmt.Errorf("unreadable event: %s", firstChars(line, 120))
		}
		if current.Action == "fail" && current.Test == "" {
			found.PackageFailed = true
		}
		if current.Action == "fail" && current.Test != "" {
			// Kept for the one case where no record explains a red engine: a
			// test that failed before its case could publish anything. Its
			// name is then the whole diagnostic, and "no case reporting a
			// failure" without it sent the last reader looking for nothing.
			found.FailedTests = append(found.FailedTests, current.Test)
		}
		if current.Action != "output" {
			continue
		}
		key := current.Package + "\x00" + current.Test
		text := partial[key] + current.Output
		for {
			end := strings.IndexByte(text, '\n')
			if end < 0 {
				break
			}
			if err := found.take(text[:end]); err != nil {
				return nil, err
			}
			text = text[end+1:]
		}
		partial[key] = text
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading the stream: %w", err)
	}
	// A record left without its newline was cut short by the end of the
	// stream, which means the run did not finish printing it. Refused rather
	// than guessed at — including a cut that landed inside the tag, which is
	// caught by asking the other way round: is what is left the beginning of a
	// tag? The length floor keeps a stray "w" from ending a healthy run, and
	// leaves a gap no shorter than that: a stream cut after eight characters
	// of a tag is not recognized. Nothing shorter has ever been produced, and
	// a floor is cheaper than being wrong about a run that was fine.
	for _, text := range partial {
		if truncatedRecord(strings.TrimSpace(text)) {
			return nil, fmt.Errorf("a record was cut short by the end of the stream: %s", firstChars(text, 120))
		}
	}
	if !found.sawRun {
		return nil, errors.New("the stream carried no run record; was this a workflow suite run?")
	}
	return found, nil
}

// take reads one line of test output and keeps it only if it is a record.
func (s *summary) take(output string) error {
	line := strings.TrimSpace(output)
	switch {
	case strings.HasPrefix(line, record.CaseMark):
		var one record.Case
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, record.CaseMark)), &one); err != nil {
			return fmt.Errorf("unreadable case record: %w", err)
		}
		if one.Case == "" || one.Outcome == "" {
			return fmt.Errorf("a case record without a name or an outcome: %s", firstChars(line, 120))
		}
		s.Cases = append(s.Cases, one)
	case strings.HasPrefix(line, record.RunMark):
		var run record.Run
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, record.RunMark)), &run); err != nil {
			return fmt.Errorf("unreadable run record: %w", err)
		}
		s.Run = run
		s.sawRun = true
	}
	return nil
}

// truncatedRecord reports whether what the stream ended with is the beginning
// of a record: a whole tag with part of its payload, or part of the tag itself.
func truncatedRecord(text string) bool {
	if strings.HasPrefix(text, record.CaseMark) || strings.HasPrefix(text, record.RunMark) {
		return true
	}
	const floor = 9 // "workflow-", the shortest leftover worth suspecting
	if len(text) < floor {
		return false
	}
	return isPrefixOf(text, record.CaseMark) || isPrefixOf(text, record.RunMark)
}

// isPrefixOf asks the question the other way round from strings.HasPrefix: is
// the short text the beginning of the long one? Written as a function because
// the reversed call reads as a mistake at the call site, and a linter agrees.
func isPrefixOf(short, long string) bool { return strings.HasPrefix(long, short) }

func firstChars(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "…"
}
