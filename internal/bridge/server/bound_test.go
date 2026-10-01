package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/bridge"
)

// The bound test (docs/mail-bridge-checks.md): the whole envelope through the
// encoder, and frames through the reader, at and past their bounds.

// fuzzID is an id built from a seed: a string of n bytes encoded, or an
// integer, or something of another kind.
func fuzzID(kind byte, text string) json.RawMessage {
	switch kind % 3 {
	case 0:
		encoded, _ := json.Marshal(text)
		return encoded
	case 1:
		return json.RawMessage(strconv.Itoa(len(text)*7919 - 4000))
	default:
		return json.RawMessage(text)
	}
}

func FuzzEveryMessageWrittenFits(f *testing.F) {
	dense := strings.Repeat("\x01", 900)
	for _, seed := range []struct {
		kind           byte
		id             string
		stdout, stderr string
		code           int
	}{
		{0, strings.Repeat("i", 126), "plain ascii\n", "", 0},
		{0, strings.Repeat("i", 127), "past the id's bound", "", 0},
		{0, strings.Repeat("ж", 63), strings.Repeat("кириллица ", 300), "предупреждение\n", 1},
		{1, "7", strings.Repeat("🙂", 900), strings.Repeat("🙂", 100), 2},
		{0, "\"\\\n", strings.Repeat(`"\`, 1500), "", 0},
		{0, strings.Repeat("\x01", 21), dense, dense, 3},
		{2, `{"n":1}`, "an id of another kind", "", 0},
		{0, "id", strings.Repeat("x", bridge.ResultCap-90), "", 0},
		{0, "id", strings.Repeat("x", bridge.ResultCap), "", 0},
		{0, strings.Repeat("i", 126), strings.Repeat("<>&", 800), "  ", 255},
	} {
		f.Add(seed.kind, seed.id, seed.stdout, seed.stderr, seed.code)
	}
	f.Fuzz(func(t *testing.T, kind byte, idText, stdout, stderr string, code int) {
		id := fuzzID(kind, idText)
		var out bytes.Buffer
		e := newEncoder(&out)
		if !validID(id) {
			// An id an answer cannot carry is answered with id null.
			e.fail(nil, codeInvalidRequest, "the request id is not one an answer can carry")
		} else {
			e.reply(id, answer{stdout: stdout, stderr: stderr, code: code})
			e.fail(id, codeInvalidParams, "a refusal of "+stdout[:min(len(stdout), 40)])
		}
		lines := bytes.Split(bytes.TrimSuffix(out.Bytes(), []byte("\n")), []byte("\n"))
		if len(lines) == 0 || len(lines[0]) == 0 {
			t.Fatal("nothing was written")
		}
		for _, line := range lines {
			if len(line) > bridge.ResultCap {
				t.Fatalf("a message of %d bytes", len(line))
			}
			var written struct {
				ID     json.RawMessage `json:"id"`
				Result *result         `json:"result"`
			}
			if err := json.Unmarshal(line, &written); err != nil {
				t.Fatalf("a message that does not parse: %v", err)
			}
			if validID(id) && !bytes.Equal(written.ID, id) {
				t.Fatalf("the id %s came back as %s", id, written.ID)
			}
			if written.Result == nil {
				continue
			}
			// A result is the child's answer byte for byte, or the
			// replacement, an error.
			first := written.Result.Content[0].Text
			if first == overBound {
				if !written.Result.IsError {
					t.Fatal("the replacement is not an error")
				}
				continue
			}
			if first != strings.ToValidUTF8(stdout, "�") || written.Result.IsError != (code != 0) {
				t.Fatalf("the answer changed on the way: %q", first)
			}
		}
	})
}

func FuzzFramesAreBounded(f *testing.F) {
	for _, seed := range []struct {
		size int
		fill string
	}{
		{1, "x"}, {maxFrame, "x"}, {maxFrame + 1, "x"}, {maxFrame - 1, "ж"}, {maxFrame + 3, "🙂"}, {70 << 10, "\r"},
	} {
		f.Add(seed.size, seed.fill)
	}
	f.Fuzz(func(t *testing.T, size int, fill string) {
		if fill == "" || strings.ContainsAny(fill, "\n") || size < 1 || size > 2*maxFrame {
			return
		}
		long := strings.Repeat(fill, size/len(fill)+1)[:size]
		reader := newFrames(io.MultiReader(strings.NewReader(long+"\n"), strings.NewReader("after\n")))
		frame, err := reader.next()
		switch {
		case size > maxFrame:
			if !errors.Is(err, errFrameTooLong) {
				t.Fatalf("a frame of %d bytes: %v, %d bytes", size, err, len(frame))
			}
		case strings.TrimSpace(long) == "":
			// A blank line is no frame: the next one is read.
			if string(frame) != "after" {
				t.Fatalf("after a blank line: %q, %v", frame, err)
			}
			return
		default:
			if err != nil || len(frame) > maxFrame || !strings.HasPrefix(long, string(frame)) {
				t.Fatalf("a frame of %d bytes: %v, %d bytes", size, err, len(frame))
			}
		}
		if after, err := reader.next(); err != nil || string(after) != "after" {
			t.Fatalf("the reader did not go on: %q, %v", after, err)
		}
	})
}
