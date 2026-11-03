package pi

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
)

// pi's tools in the face's words (docs/face.md, "Calls"; research-pi.md): the
// face draws every harness as Claude Code's terminal looks and knows no
// harness, so each of pi's built-in tools is named by its Claude Code kin and
// its input given in that tool's fields, each form a row of the face's table
// (face/src/call.cases.ts, held by translate_test.go). A tool with no kin
// keeps its own name and input, which the face draws by its generic rule.

// faced is a call as the face reads it: its name and its input.
type faced struct {
	name  string
	input any
}

// fields is an object whose keys keep their order, as the kin names them.
type fields []field

type field struct {
	key   string
	value any
}

func (f fields) MarshalJSON() ([]byte, error) {
	var out bytes.Buffer
	out.WriteByte('{')
	for i, one := range f {
		if i > 0 {
			out.WriteByte(',')
		}
		key, _ := json.Marshal(one.key)
		value, err := json.Marshal(one.value)
		if err != nil {
			return nil, err
		}
		out.Write(key)
		out.WriteByte(':')
		out.Write(value)
	}
	out.WriteByte('}')
	return out.Bytes(), nil
}

// given is what args say of keys, renamed, in the kin's order.
func given(args map[string]json.RawMessage, keys ...string) fields {
	out := fields{}
	for i := 0; i+1 < len(keys); i += 2 {
		if value, ok := args[keys[i]]; ok {
			out = append(out, field{keys[i+1], value})
		}
	}
	return out
}

var unquoted = regexp.MustCompile(`^[\w./~@%+=:,-]+$`)

// quoted is a path as a shell reads it, quoted only when it must be.
func quoted(path string) string {
	if unquoted.MatchString(path) {
		return path
	}
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

func facedTool(name string, raw json.RawMessage) faced {
	var args map[string]json.RawMessage
	object := json.Unmarshal(raw, &args) == nil && args != nil
	switch name {
	case "bash":
		return faced{"Bash", given(args, "command", "command")}
	case "read":
		return faced{"Read", given(args, "path", "file_path", "offset", "offset", "limit", "limit")}
	case "write":
		return faced{"Write", given(args, "path", "file_path", "content", "content")}
	case "edit":
		var edits []struct {
			OldText any `json:"oldText"`
			NewText any `json:"newText"`
		}
		_ = json.Unmarshal(args["edits"], &edits)
		pairs := make([]fields, 0, len(edits))
		for _, e := range edits {
			old, _ := e.OldText.(string)
			replaced, _ := e.NewText.(string)
			pairs = append(pairs, fields{{"old_string", old}, {"new_string", replaced}})
		}
		input := given(args, "path", "file_path")
		// One edit is Claude Code's Edit; several, the edits of its MultiEdit.
		if len(pairs) == 1 {
			return faced{"Edit", append(input, pairs[0]...)}
		}
		return faced{"MultiEdit", append(input, field{"edits", pairs})}
	case "grep":
		return faced{"Grep", given(args, "pattern", "pattern", "path", "path", "glob", "glob", "ignoreCase", "-i", "context", "-C", "limit", "head_limit")}
	case "find":
		return faced{"Glob", given(args, "pattern", "pattern", "path", "path")}
	case "ls":
		// Claude Code has no tool of its own to list a directory (2.1.310): it
		// runs ls in its shell, which the face reads as a listed directory.
		var path string
		_ = json.Unmarshal(args["path"], &path)
		command := "ls"
		if path != "" {
			command += " " + quoted(path)
		}
		return faced{"Bash", fields{{"command", command}}}
	}
	if !object {
		return faced{name, fields{}}
	}
	return faced{name, raw}
}

// resultText is a tool's result as the face draws its output: the text of
// its content, an image named as one.
func resultText(raw json.RawMessage) string {
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text any    `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(raw, &result) != nil || result.Content == nil {
		var text string
		_ = json.Unmarshal(raw, &text)
		return text
	}
	var texts []string
	for _, part := range result.Content {
		if text, ok := part.Text.(string); ok && part.Type == "text" && text != "" {
			texts = append(texts, text)
		} else if part.Type == "image" {
			texts = append(texts, "[image]")
		}
	}
	return strings.Join(texts, "\n")
}

// textOf is a user message's text as pi matches it against its queue: its
// text parts joined with nothing between.
func textOf(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var content []struct {
		Type string `json:"type"`
		Text any    `json:"text"`
	}
	_ = json.Unmarshal(raw, &content)
	var out strings.Builder
	for _, part := range content {
		if text, ok := part.Text.(string); ok && part.Type == "text" {
			out.WriteString(text)
		}
	}
	return out.String()
}

func stringOf(raw json.RawMessage) string {
	var text string
	_ = json.Unmarshal(raw, &text)
	return text
}

func blank(text string) bool { return strings.TrimSpace(text) == "" }
