package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/receipt"
	"github.com/praline-labs/rewake/internal/state"
)

// An answer longer than one tool result — a help page, a long --owed — is
// kept in the journal and handed out a part per call. Nothing is marked by
// it: only a read of letters has a boundary to cross, and that has its own
// path (inbox_parts.go).

// nextToken names where a continuation starts: a record, a letter of it (0
// for an output), and a part.
func nextToken(token string, letter, part int) string {
	return fmt.Sprintf("%s.%d.%d", token, letter, part)
}

// parseNext reads a continuation token back. It selects a record of this run
// and a position in it; it is no path and no authority over another run.
func parseNext(raw string) (string, int, int, bool) {
	fields := strings.Split(raw, ".")
	if len(fields) != 3 || !receipt.ValidToken(fields[0]) {
		return "", 0, 0, false
	}
	letter, errLetter := strconv.Atoi(fields[1])
	part, errPart := strconv.Atoi(fields[2])
	if errLetter != nil || errPart != nil || letter < 0 || part < 0 || letter > 1<<16 || part > 1<<16 {
		return "", 0, 0, false
	}
	return fields[0], letter, part, true
}

// nextWords are the exact words that fetch a continuation.
func nextWords(token string, letter, part int) []string {
	return []string{"inbox", "--next", nextToken(token, letter, part)}
}

// outputPartModel is one part of a kept output under --json: a whole envelope,
// whatever it carries a slice of.
type outputPartModel struct {
	Receipt   string   `json:"receipt"`
	Part      int      `json:"part"`
	Parts     int      `json:"parts"`
	Text      string   `json:"text"`
	NextWords []string `json:"nextWords,omitempty"`
}

// freezeOutput keeps an output that does not fit one result and returns its
// first part.
func freezeOutput(ctx *Context, out string) (string, error) {
	if ctx.scope == nil {
		return "", errors.New("no tool call to keep it for")
	}
	dir, err := state.Dir()
	if err != nil {
		return "", err
	}
	self, epoch, err := ownRun(dir)
	if err != nil {
		return "", err
	}
	frozen := &receipt.FrozenText{JSON: ctx.JSON, Text: out}
	// Sized as if the longest diagnostic went along with every part: the
	// first carries it, and the rest then fit all the more.
	reserve := strings.Repeat("x", diagnosticCap+100)
	sizing := receipt.Record{Token: strings.Repeat("0", 24)}
	for start := 0; start < len(out); {
		end := bridge.Cut(out, start, func(end int) bool {
			sizing.Output = &receipt.FrozenText{JSON: frozen.JSON, Text: out, Parts: []receipt.Part{{Start: start, End: end}}}
			return bridge.Fits(renderOutputPart(sizing, 0, 9999), reserve)
		})
		if end <= start {
			return "", errors.New("a part would not fit one result")
		}
		frozen.Parts = append(frozen.Parts, receipt.Part{Start: start, End: end})
		start = end
	}
	record, _, err := receipt.Begin(dir, self.Name, receipt.Key{Epoch: epoch, Digest: ctx.scope.digest},
		receipt.Record{Words: ctx.scope.words, Transport: ctx.scope.ticket.Transport, CalledBoot: ctx.scope.ticket.CalledBoot, Output: frozen})
	if err != nil {
		return "", err
	}
	record.Phase = receipt.Done
	if err := receipt.Save(dir, self.Name, record); err != nil {
		return "", err
	}
	return renderOutputPart(record, 0, len(frozen.Parts)), nil
}

// renderOutputPart prints part index of a kept output. count is the number of
// parts, given apart so a part can be sized before the count is known.
func renderOutputPart(record receipt.Record, index, count int) string {
	frozen := record.Output
	part := frozen.Parts[index]
	text := frozen.Text[part.Start:part.End]
	var next []string
	if index+1 < count {
		next = nextWords(record.Token, 0, index+1)
	}
	if frozen.JSON {
		encoded, _ := json.Marshal(outputPartModel{Receipt: record.Token, Part: index + 1, Parts: count, Text: text, NextWords: next})
		return string(encoded) + "\n"
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if next == nil {
		return text + fmt.Sprintf("Rewake: part %d of %d, the end of this output.\n", index+1, count)
	}
	return text + fmt.Sprintf("Rewake: part %d of %d of this output; the next: rewake %s\n", index+1, count, strings.Join(next, " "))
}

// continueOutput prints the part of an earlier output or read a token names.
// It works from the shell as from the tool: a tool that lost its answer
// leaves the shell to fetch the rest.
func continueOutput(ctx *Context, call Call, raw string) error {
	token, letter, part, ok := parseNext(raw)
	if !ok {
		return &UsageError{Command: call.Command, Message: "--next needs the token a partial output printed, as in its next words."}
	}
	dir, err := state.Dir()
	if err != nil {
		return &UsageError{Command: call.Command, Message: err.Error()}
	}
	self, epoch, err := ownRun(dir)
	if err != nil {
		return failf("%v; a continuation belongs to the run that printed it", err)
	}
	record, err := receipt.Load(dir, self.Name, epoch, token)
	if errors.Is(err, receipt.ErrUnknown) {
		return failf("no output of this run answers to %s: a token names an output of the run that printed it, kept for a day", raw)
	}
	if err != nil {
		return failf("could not read the output %s: %v", token, err)
	}
	if record.Read != nil {
		if ctx.JSON != record.Read.JSON && ctx.JSON {
			return &UsageError{Command: call.Command, Message: "this read was printed as text and continues so; drop --json."}
		}
		ctx.JSON = record.Read.JSON
		return emitRead(ctx, readSite{dir: dir, self: self, epoch: epoch}, record.Token, letter, part)
	}
	if record.Output == nil || letter != 0 || part >= len(record.Output.Parts) {
		return failf("%s names no part of a kept output; take the token from the words the output printed", raw)
	}
	if ctx.JSON != record.Output.JSON && ctx.JSON {
		return &UsageError{Command: call.Command, Message: "this output was printed as text and continues so; drop --json."}
	}
	_, err = fmt.Fprint(ctx.Stdout, renderOutputPart(record, part, len(record.Output.Parts)))
	return err
}
