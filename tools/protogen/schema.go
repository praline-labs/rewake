package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
)

// schema is the protocol as both sides are generated from it. Definitions,
// fields and endpoints keep the schema's order, so the generated files read in
// the order the schema was written in.
type schema struct {
	// source is the schema file's name and goPackage the package of the Go
	// side, both set by whoever reads the file.
	source, goPackage  string
	title, description string
	bounds             []bound
	endpoints          []endpoint
	defs               []def
}

type bound struct {
	name, description string
	value             int
}

type endpoint struct {
	path, description string
	request, answer   string // names of object definitions
}

// def is a named definition: an object with its fields, or a string enum.
type def struct {
	name, description string
	enum              []string // set for an enum, nil for an object
	fields            []field
}

type field struct {
	name, description string
	required          bool
	shape             shape
}

// shape is what a field holds: a scalar or a definition, or an array of one.
type shape struct {
	scalar string // "string", "integer" or "boolean"; empty for a reference
	ref    string // a definition's name; empty for a scalar
	array  bool
}

// The JSON words each part may use; json's DisallowUnknownFields refuses the
// rest. Kind carries the word "type".
type (
	rawSchema struct {
		Schema      string          `json:"$schema"`
		Title       string          `json:"title"`
		Description string          `json:"description"`
		Bounds      json.RawMessage `json:"x-bounds"`
		Endpoints   []rawEndpoint   `json:"x-endpoints"`
		Defs        json.RawMessage `json:"$defs"`
	}
	rawEndpoint struct {
		Path        string `json:"path"`
		Description string `json:"description"`
		Request     rawRef `json:"request"`
		Answer      rawRef `json:"answer"`
	}
	rawRef struct {
		Ref string `json:"$ref"`
	}
	rawBound struct {
		Value       *int   `json:"value"`
		Description string `json:"description"`
	}
	rawNode struct {
		Ref                  string          `json:"$ref"`
		Kind                 string          `json:"type"`
		Description          string          `json:"description"`
		Enum                 []string        `json:"enum"`
		Properties           json.RawMessage `json:"properties"`
		Required             []string        `json:"required"`
		AdditionalProperties *bool           `json:"additionalProperties"`
		Items                json.RawMessage `json:"items"`
	}
)

var (
	defName   = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`)
	fieldName = regexp.MustCompile(`^[a-z][A-Za-z0-9]*$`)
	enumValue = regexp.MustCompile(`^[a-z][a-z0-9]*$`)
	pathName  = regexp.MustCompile(`^[a-z]+(/[a-z]+)*$`)
)

const refPrefix = "#/$defs/"

// parseSchema reads the subset of JSON Schema the protocol uses and refuses
// anything else, naming where.
func parseSchema(text []byte) (schema, error) {
	var raw rawSchema
	if err := strict(text, &raw); err != nil {
		return schema{}, err
	}
	if raw.Title == "" || raw.Description == "" {
		return schema{}, errors.New("the schema has a title and a description")
	}
	out := schema{title: raw.Title, description: raw.Description}
	// A schema of types alone, like the room view's, has no bounds and no
	// endpoints; one that names them names them whole.
	var bounds []member
	if raw.Bounds != nil {
		var err error
		if bounds, err = members(raw.Bounds, "x-bounds"); err != nil {
			return schema{}, err
		}
	}
	for _, m := range bounds {
		var b rawBound
		if err := strict(m.value, &b); err != nil {
			return schema{}, fmt.Errorf("x-bounds.%s: %w", m.key, err)
		}
		if !fieldName.MatchString(m.key) || b.Value == nil || b.Description == "" {
			return schema{}, fmt.Errorf("x-bounds.%s: a bound is a lower camel name with a value and a description", m.key)
		}
		out.bounds = append(out.bounds, bound{name: m.key, value: *b.Value, description: b.Description})
	}
	defs, err := members(raw.Defs, "$defs")
	if err != nil {
		return schema{}, err
	}
	for _, m := range defs {
		d, err := parseDef(m.key, m.value)
		if err != nil {
			return schema{}, fmt.Errorf("$defs.%s: %w", m.key, err)
		}
		out.defs = append(out.defs, d)
	}
	if err := out.checkRefs(); err != nil {
		return schema{}, err
	}
	if raw.Endpoints != nil && len(raw.Endpoints) == 0 {
		return schema{}, errors.New("x-endpoints names no endpoint")
	}
	for _, e := range raw.Endpoints {
		parsed, err := out.parseEndpoint(e)
		if err != nil {
			return schema{}, fmt.Errorf("x-endpoints %q: %w", e.Path, err)
		}
		out.endpoints = append(out.endpoints, parsed)
	}
	return out, nil
}

func parseDef(name string, text json.RawMessage) (def, error) {
	if !defName.MatchString(name) {
		return def{}, errors.New("a definition's name is upper camel")
	}
	var node rawNode
	if err := strict(text, &node); err != nil {
		return def{}, err
	}
	d := def{name: name, description: node.Description}
	if d.description == "" {
		return def{}, errors.New("a definition has a description")
	}
	switch {
	case node.Kind == "string" && node.Enum != nil:
		if node.Ref != "" || node.Properties != nil || node.Required != nil || node.AdditionalProperties != nil || node.Items != nil {
			return def{}, errors.New("an enum has a type, a description and its values, nothing else")
		}
		if len(node.Enum) == 0 {
			return def{}, errors.New("an enum has values")
		}
		for i, value := range node.Enum {
			if !enumValue.MatchString(value) || slices.Contains(node.Enum[:i], value) {
				return def{}, fmt.Errorf("an enum's value is a distinct lower-case word, not %q", value)
			}
		}
		d.enum = node.Enum
		return d, nil
	case node.Kind == "object":
		fields, err := parseObject(node)
		d.fields = fields
		return d, err
	}
	return def{}, errors.New("a definition is an object or a string enum")
}

// parseObject reads an object's fields. Every object is closed and says which
// fields it requires, so the generated types are the whole message.
func parseObject(node rawNode) ([]field, error) {
	if node.Ref != "" || node.Enum != nil || node.Items != nil {
		return nil, errors.New("an object has properties, required and additionalProperties, nothing else")
	}
	if node.AdditionalProperties == nil || *node.AdditionalProperties || node.Properties == nil || node.Required == nil {
		return nil, errors.New("an object names its properties and its required ones, and sets additionalProperties to false")
	}
	properties, err := members(node.Properties, "properties")
	if err != nil {
		return nil, err
	}
	var fields []field
	for _, m := range properties {
		f, err := parseField(m.key, m.value)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", m.key, err)
		}
		fields = append(fields, f)
	}
	for i, name := range node.Required {
		at := slices.IndexFunc(fields, func(f field) bool { return f.name == name })
		if at < 0 || slices.Contains(node.Required[:i], name) {
			return nil, fmt.Errorf("required names %q, which is no property or named twice", name)
		}
		fields[at].required = true
	}
	return fields, nil
}

func parseField(name string, text json.RawMessage) (field, error) {
	if !fieldName.MatchString(name) {
		return field{}, errors.New("a property's name is lower camel")
	}
	var node rawNode
	if err := strict(text, &node); err != nil {
		return field{}, err
	}
	if node.Description == "" {
		return field{}, errors.New("a property has a description")
	}
	f := field{name: name, description: node.Description}
	if node.Kind == "array" {
		if node.Items == nil || node.Ref != "" || node.Enum != nil || node.Properties != nil || node.Required != nil || node.AdditionalProperties != nil {
			return field{}, errors.New("an array property has a type, items and a description, nothing else")
		}
		var item rawNode
		if err := strict(node.Items, &item); err != nil {
			return field{}, fmt.Errorf("items: %w", err)
		}
		shape, err := parseShape(item)
		if err != nil {
			return field{}, fmt.Errorf("items: %w", err)
		}
		shape.array = true
		f.shape = shape
		return f, nil
	}
	shape, err := parseShape(node)
	f.shape = shape
	return f, err
}

// parseShape reads a scalar or a reference. An object or an enum written in
// place is refused: it is a definition, so both sides name it alike.
func parseShape(node rawNode) (shape, error) {
	if node.Enum != nil || node.Properties != nil || node.Required != nil || node.AdditionalProperties != nil || node.Items != nil {
		return shape{}, errors.New("an object, an enum or an array is a definition of its own, referenced by $ref")
	}
	switch {
	case node.Ref != "" && node.Kind == "":
		name, ok := strings.CutPrefix(node.Ref, refPrefix)
		if !ok {
			return shape{}, fmt.Errorf("a reference names %s<name>, not %q", refPrefix, node.Ref)
		}
		return shape{ref: name}, nil
	case node.Ref == "" && (node.Kind == "string" || node.Kind == "integer" || node.Kind == "boolean"):
		return shape{scalar: node.Kind}, nil
	}
	return shape{}, errors.New("a property is a string, an integer, a boolean, an array or a $ref")
}

func (s schema) find(name string) (def, bool) {
	at := slices.IndexFunc(s.defs, func(d def) bool { return d.name == name })
	if at < 0 {
		return def{}, false
	}
	return s.defs[at], true
}

func (s schema) checkRefs() error {
	for _, d := range s.defs {
		for _, f := range d.fields {
			if _, ok := s.find(f.shape.ref); f.shape.ref != "" && !ok {
				return fmt.Errorf("$defs.%s.%s: a reference to %q, which is no definition", d.name, f.name, f.shape.ref)
			}
		}
	}
	return nil
}

func (s schema) parseEndpoint(e rawEndpoint) (endpoint, error) {
	if !pathName.MatchString(e.Path) || e.Description == "" {
		return endpoint{}, errors.New("an endpoint has a path of lower-case words and a description")
	}
	if slices.ContainsFunc(s.endpoints, func(other endpoint) bool { return other.path == e.Path }) {
		return endpoint{}, errors.New("an endpoint is named once")
	}
	out := endpoint{path: e.Path, description: e.Description}
	for _, part := range []struct {
		ref  string
		into *string
	}{{e.Request.Ref, &out.request}, {e.Answer.Ref, &out.answer}} {
		name, local := strings.CutPrefix(part.ref, refPrefix)
		if !local {
			return endpoint{}, fmt.Errorf("a request and an answer reference %s<name>, not %q", refPrefix, part.ref)
		}
		if d, ok := s.find(name); !ok || d.enum != nil {
			return endpoint{}, fmt.Errorf("a request and an answer reference object definitions, not %q", part.ref)
		}
		*part.into = name
	}
	return out, nil
}

// strict decodes one value, refusing words its struct does not name and
// anything after the value: a second decode must meet the end of the text,
// since More answers only inside an array or an object and would pass a stray
// closing delimiter. Nested raw values are decoded strictly by their own call.
func strict(text []byte, into any) error {
	decoder := json.NewDecoder(bytes.NewReader(text))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return err
	}
	var rest json.RawMessage
	if err := decoder.Decode(&rest); !errors.Is(err, io.EOF) {
		return errors.New("text after the value")
	}
	return nil
}

type member struct {
	key   string
	value json.RawMessage
}

// members reads a JSON object's members in their order, which a Go map loses.
func members(text json.RawMessage, what string) ([]member, error) {
	if text == nil {
		return nil, fmt.Errorf("%s is missing", what)
	}
	decoder := json.NewDecoder(bytes.NewReader(text))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return nil, fmt.Errorf("%s is an object", what)
	}
	var out []member
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", what, err)
		}
		key, _ := token.(string)
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("%s.%s: %w", what, key, err)
		}
		if slices.ContainsFunc(out, func(m member) bool { return m.key == key }) {
			return nil, fmt.Errorf("%s names %q twice", what, key)
		}
		out = append(out, member{key: key, value: value})
	}
	return out, nil
}
