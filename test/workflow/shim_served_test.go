package workflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// What a harness program of the suite accepts: a closed shape for each value
// it is handed, so a field nobody modeled is refused rather than passed.

// jsonKind reports what a raw JSON value is, or "" when it is unreadable.
// "" is a kind of its own here rather than a missing answer: it equals none of
// the kinds a description asks for, so an unreadable value is refused wherever
// a kind is compared.
func jsonKind(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || !json.Valid(trimmed) {
		return ""
	}
	switch trimmed[0] {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	default:
		return "number"
	}
}

// describeKind names what arrived, for a refusal that says what was wrong.
func describeKind(raw json.RawMessage) string {
	if kind := jsonKind(raw); kind != "" {
		return kind
	}
	return "something unreadable"
}

// served describes one value the fixture accepts.
type served struct {
	// kind is the JSON kind the value must have.
	kind string
	// fields is the closed set for an object. An object with no fields listed
	// is one whose contents this fixture does not inspect — and there are none
	// of those in the delivery path on purpose.
	fields map[string]served
	// required names the fields an object must carry. A closed set says what
	// may be there; this says what has to be.
	required []string
	// items is what every element of an array must look like.
	items *served
	// nullable says JSON null stands for the value left out, as the protocol's
	// optional fields allow.
	nullable bool
	// text is a further condition on a string, beyond being a string. The
	// protocol carries meanings a JSON kind cannot express — a workspace root
	// is a path, and the server refuses a relative one — and a check that
	// stops at "it is a string" is the same surface-level pass that let a
	// nested object through.
	text *textRule
}

// textRule is a named condition on a string, so a refusal can say which one
// was not met.
type textRule struct {
	name string
	ok   func(string) bool
}

// unserved names the first thing wrong with a value, or "" when the fixture
// serves it. Every problem it reports is a non-empty sentence naming a path,
// so the empty answer cannot be one of them. The path names where the problem is, because "must be a string"
// about an unnamed field several levels down helps nobody.
func unserved(path string, raw json.RawMessage, want served) string {
	kind := jsonKind(raw)
	if kind == "null" && want.nullable {
		return ""
	}
	if kind != want.kind {
		return fmt.Sprintf("%s must be a %s, got %s", path, want.kind, describeKind(raw))
	}
	switch want.kind {
	case "string":
		if want.text == nil {
			return ""
		}
		var value string
		if json.Unmarshal(raw, &value) != nil {
			return path + " is not a readable string"
		}
		if !want.text.ok(value) {
			return fmt.Sprintf("%s must be %s, got %q", path, want.text.name, value)
		}
	case "object":
		if want.fields == nil {
			return ""
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil {
			return path + " is not a readable object"
		}
		for _, name := range want.required {
			if _, ok := fields[name]; !ok {
				return fmt.Sprintf("%s is missing %q", path, name)
			}
		}
		for _, name := range sortedKeys(fields) {
			inner, ok := want.fields[name]
			if !ok {
				return fmt.Sprintf("this fixture does not serve %q on %s", name, path)
			}
			if problem := unserved(path+"."+name, fields[name], inner); problem != "" {
				return problem
			}
		}
	case "array":
		if want.items == nil {
			return ""
		}
		var elements []json.RawMessage
		if json.Unmarshal(raw, &elements) != nil {
			return path + " is not a readable list"
		}
		for i, element := range elements {
			if problem := unserved(fmt.Sprintf("%s[%d]", path, i), element, *want.items); problem != "" {
				return problem
			}
		}
	}
	return ""
}

// sortedKeys keeps a refusal from depending on map order: the same request has
// to be refused for the same reason every time, or a control checking the
// reason becomes flaky.
func sortedKeys(fields map[string]json.RawMessage) []string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// repeatedKey names the first field that appears twice in the same object,
// anywhere in the value, or "" when every name is unique.
//
// JSON allows a repeated name and says nothing about which one wins, so every
// reader is free to choose — and two readers of ours chose differently:
// decoding into a struct kept the earlier object's fields, a map kept the
// later one's, and a required field could go missing between the two views.
// The fixture refuses such a request instead of picking a side, which also
// keeps the next reader somebody writes from quietly choosing a third.
func repeatedKey(raw json.RawMessage) (string, bool, error) {
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	// Numbers stay as they were written. The default turns every number into a
	// float64, which fails on one too large to represent — and a scan that
	// ended in a failure used to return "no repetition found", so a request
	// could carry both a huge number and a repeated key and pass. Not being
	// able to look is not the same as having looked.
	decoder.UseNumber()
	// One frame per open object or array. Inside an object, names and values
	// alternate, and a nested value hands the turn back when it closes — which
	// is the part a flat flag got wrong, leaving every name after the first
	// one unchecked.
	type frame struct {
		object    bool
		expectKey bool
		seen      map[string]bool
	}
	var stack []frame
	value := func() {
		if len(stack) > 0 && stack[len(stack)-1].object {
			stack[len(stack)-1].expectKey = true
		}
	}
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			if len(stack) > 0 {
				// The input ended inside an object or a list. Reaching the end
				// of a truncated document is not the same as having read the
				// whole of it, and saying otherwise would be the same mistake
				// one level up.
				return "", false, errors.New("the value ended in the middle")
			}
			// The whole value was scanned and every name was unique.
			return "", false, nil
		}
		if err != nil {
			return "", false, err
		}
		if delimiter, ok := token.(json.Delim); ok {
			switch delimiter {
			case '{':
				stack = append(stack, frame{object: true, expectKey: true, seen: map[string]bool{}})
			case '[':
				stack = append(stack, frame{})
			case '}', ']':
				stack = stack[:len(stack)-1]
				value()
			}
			continue
		}
		if len(stack) == 0 || !stack[len(stack)-1].object {
			continue
		}
		current := &stack[len(stack)-1]
		if !current.expectKey {
			current.expectKey = true
			continue
		}
		name, isName := token.(string)
		current.expectKey = false
		if !isName {
			continue
		}
		if current.seen[name] {
			// found is a separate answer from the name: a repeated key may be
			// the empty string, and a caller reading emptiness as "nothing
			// found" walks straight past it. Two readers of ours already made
			// that mistake with an error; this is the same shape.
			return name, true, nil
		}
		current.seen[name] = true
	}
}

// singleNames is how a caller asks the question: it returns the problem to
// report, and an empty string only when the value was read to the end and
// carried no repeated name.
//
// The shape of this pair is deliberate. The first version answered with a name
// and nothing else, so a scan that could not finish looked exactly like a scan
// that found nothing — the same "could not check, therefore fine" this suite
// exists to prevent, this time inside a check.
func singleNames(path string, raw json.RawMessage) string {
	repeated, found, err := repeatedKey(raw)
	if err != nil {
		return fmt.Sprintf("%s could not be read to the end: %v", path, err)
	}
	if found {
		return fmt.Sprintf("%s carries %q twice", path, repeated)
	}
	return ""
}
