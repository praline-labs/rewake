package pi

import (
	"encoding/json"
	"strings"
)

// partialJSON is the object a call's streamed arguments say so far, as
// pi-ai's own parse of a stream reads one: an open string value is closed
// where it stops, and an unfinished key, number or literal is left out.
// What cannot be read at all is an empty object.
func partialJSON(prefix string) json.RawMessage {
	var stack []byte
	// safe is the longest prefix that is a document once closed; closers
	// closes it.
	safe, closers := 0, ""
	inString, escaped, key := false, false, false
	// expectKey is per object level: the next string is a key.
	var expectKey []bool
	closing := func() string {
		var out strings.Builder
		for i := len(stack) - 1; i >= 0; i-- {
			if stack[i] == '{' {
				out.WriteByte('}')
			} else {
				out.WriteByte(']')
			}
		}
		return out.String()
	}
	keep := func(at int) { safe, closers = at, closing() }
	for i := 0; i < len(prefix); i++ {
		c := prefix[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
				if !key {
					keep(i + 1)
				}
			}
			continue
		}
		switch c {
		case '{', '[':
			stack = append(stack, c)
			expectKey = append(expectKey, c == '{')
			keep(i + 1)
		case '}', ']':
			if len(stack) == 0 {
				return json.RawMessage("{}")
			}
			stack, expectKey = stack[:len(stack)-1], expectKey[:len(expectKey)-1]
			keep(i + 1)
		case '"':
			inString, key = true, len(expectKey) > 0 && expectKey[len(expectKey)-1]
			if key {
				expectKey[len(expectKey)-1] = false
			}
		case ',':
			if len(stack) > 0 && stack[len(stack)-1] == '{' {
				expectKey[len(expectKey)-1] = true
			}
		case ' ', '\t', '\n', '\r', ':':
		default:
			// A number or a literal: whole once a delimiter follows it.
			end := i
			for end < len(prefix) && !strings.ContainsRune(" \t\n\r,]}", rune(prefix[end])) {
				end++
			}
			if end < len(prefix) {
				keep(end)
			}
			i = end - 1
		}
	}
	candidates := []string{}
	if inString && !key {
		open := strings.TrimSuffix(prefix, `\`)
		if escaped && open != prefix {
			candidates = append(candidates, open+`"`+closing())
		} else if at := strings.LastIndex(prefix, `\u`); at >= 0 && len(prefix)-at < 6 {
			candidates = append(candidates, prefix[:at]+`"`+closing())
		} else {
			candidates = append(candidates, prefix+`"`+closing())
		}
	}
	if !inString {
		candidates = append(candidates, prefix+closing())
	}
	candidates = append(candidates, prefix[:safe]+closers)
	for _, candidate := range candidates {
		var object map[string]any
		if json.Unmarshal([]byte(candidate), &object) == nil && object != nil {
			return json.RawMessage(candidate)
		}
	}
	return json.RawMessage("{}")
}
