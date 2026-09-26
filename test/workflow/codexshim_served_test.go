package workflow

// What the fixture serves, described as a closed shape and checked all the way
// down.
//
// The first version of this listed the top-level fields of a delivery with the
// JSON kind of each. That stopped at the surface: `toolOutput` had to be an
// object, and anything inside it passed — so `toolOutput.namespace: true`, a
// protocol field carrying a value the real server refuses, was accepted. The
// acceptance round of September 21, 2026 found it there and named the class
// rather than the field: a check that stops at a nested object leaves every
// field inside every nested object unchecked.
//
// So the description is recursive, and the rule is the same at every level:
// a field not named here is refused by name, and a field named here must have
// the kind stated for it.
//
// The other rule this file keeps is about its own answers. Three times in one
// evening a check here said "nothing wrong" when it meant "could not tell":
// a scan that failed returned no finding, a number it could not parse ended
// the scan quietly, and a find whose name was empty looked like no find at
// all. So: a result and the absence of a result are never the same value.
// Where a function can fail, the failure is its own return; where it can find
// something whose value may be empty, the finding is its own flag. The only
// single-valued answer left is a problem description, and every problem here
// is a non-empty sentence — "" means checked and fine, and nothing else.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
)

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

// anAbsolutePath is what a workspace root must be. Verified live on 0.155.1:
// the server refuses a relative root outright.
var anAbsolutePath = &textRule{name: "an absolute path", ok: filepath.IsAbs}

// deliveryShape is what a mailbox delivery may look like, in full.
//
// `input` is an empty list for a delivery, which is checked separately because
// emptiness is not a shape; the element description is here so that a request
// carrying something is refused for the right reason.
var deliveryShape = served{kind: "object", fields: map[string]served{
	"threadId":            {kind: "string"},
	"clientUserMessageId": {kind: "string"},
	"input":               {kind: "array", items: &served{kind: "object"}},
	"toolOutput": {kind: "object", fields: map[string]served{
		"name": {kind: "string"},
		// The notice travels as a string and is checked as a notice after it
		// is parsed; see noticeShape.
		"output": {kind: "string"},
	}, required: []string{"name", "output"}},
	"runtimeWorkspaceRoots": {kind: "array", items: &served{kind: "string", text: anAbsolutePath}},
}, required: []string{"threadId", "clientUserMessageId", "input", "toolOutput"}}

// noticeShape is the payload rewake puts in that output. It is our own format,
// and it is closed for the same reason the protocol's is: a field the fixture
// does not know about is a change nobody would otherwise notice.
var noticeShape = served{kind: "object", fields: map[string]served{
	"notice": {kind: "string"},
	"members": {kind: "array", items: &served{kind: "object", fields: map[string]served{
		"id":        {kind: "string"},
		"from":      {kind: "string"},
		"fromEpoch": {kind: "string"},
		"to":        {kind: "string"},
		"toEpoch":   {kind: "string"},
		// Only on a recall: the message it tells the session not to act on.
		"recalls": {kind: "string"},
		// Only on a replacement sent by rewake edit: the message it replaces.
		"replaces": {kind: "string"},
	}, required: []string{"id", "from", "fromEpoch", "to", "toEpoch"}}},
}, required: []string{"notice", "members"}}

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

// unservedDelivery names the first thing wrong with a turn/start, notice
// included. The notice arrives as a string of JSON inside the request, so it
// is checked after parsing rather than left as an opaque blob — an object
// nobody looks inside is exactly the hole this replaced.
func unservedDelivery(params json.RawMessage, output string) string {
	// Before anything is read twice: a request with a repeated key means two
	// readers of it can disagree, and they did — decoding into a struct kept
	// the fields of the first object while a map kept the last. The protocol
	// has one value per name, so a request carrying two is refused before
	// either reader sees it.
	if problem := singleNames("the delivery", params); problem != "" {
		return problem
	}
	if problem := unserved("the delivery", params, deliveryShape); problem != "" {
		return problem
	}
	// An empty notice is refused rather than skipped. "Nothing to look at, so
	// nothing is wrong" is the same shape of answer as a scan that could not
	// finish, and a delivery whose notice is empty is not a delivery.
	if strings.TrimSpace(output) == "" {
		return "the delivery carries no notice to check"
	}
	// The notice is a document of its own, so it gets the same treatment: a
	// name repeated inside it is a name two readers would resolve differently,
	// and it travels through a string where nothing else would notice.
	notice := json.RawMessage(strings.TrimSpace(output))
	if problem := singleNames("the notice", notice); problem != "" {
		return problem
	}
	if problem := unserved("the notice", notice, noticeShape); problem != "" {
		return problem
	}
	return ""
}

// startShape and resumeShape close the other request this fixture serves. The
// nested object here is `config`, closed: an empty description would have left
// the same hole the delivery path had — an object nobody looks inside. The one
// setting served is web_search, which a terminal's builder writes into every
// lifecycle request, and it takes the four modes only. The permissions,
// and on a resume the history and the path, are served as left out, which is
// all a terminal sends of them.
var startShape = served{kind: "object", fields: map[string]served{
	"threadSource":          {kind: "string"},
	"config":                {kind: "object", fields: map[string]served{"web_search": {kind: "string", text: aWebSearchMode}}},
	"runtimeWorkspaceRoots": {kind: "array", nullable: true, items: &served{kind: "string", text: anAbsolutePath}},
	"permissions":           {kind: "null"},
}, required: []string{"threadSource"}}

var resumeShape = served{kind: "object", fields: withField(withField(withField(withField(startShape.fields,
	"threadId", served{kind: "string"}), "history", served{kind: "null"}), "path", served{kind: "null"}),
	"excludeTurns", served{kind: "boolean"})}

// goalShape is thread/goal/get, which names its conversation and nothing else.
var goalShape = served{kind: "object", fields: map[string]served{"threadId": {kind: "string"}}, required: []string{"threadId"}}

// aWebSearchMode is what web_search may be (protocol/src/config_types.rs).
var aWebSearchMode = &textRule{name: "a web_search mode", ok: func(value string) bool {
	switch value {
	case "disabled", "cached", "indexed", "live":
		return true
	}
	return false
}}

// withField copies a field set and adds one, so the two descriptions cannot
// drift apart by editing one of them.
func withField(fields map[string]served, name string, value served) map[string]served {
	copied := map[string]served{name: value}
	for key, inner := range fields {
		copied[key] = inner
	}
	return copied
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
