package workflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Checking a message against the schema.
//
// Two earlier versions claimed more than they checked: the first compared
// field names only, the second treated value constraints as decoration. The
// root was never the individual gaps — it was that "safe to ignore" got
// decided one keyword at a time, in passing.
//
// So the vocabulary is a closed register, in three categories:
//
//   - constraining: implemented, and each one has a test;
//   - annotation: ignored deliberately, and only for keywords that cannot
//     change whether a value is allowed;
//   - everything else: the check fails loudly.
//
// The last category is the point. A keyword nobody anticipated must not become
// the next quiet pass — "could not check" is never "checked".
//
// This is not a JSON Schema validator and must not become one. It covers what
// the app-server schema uses; the register below says exactly what that is.
//
// Three limits are known and deliberately left open, because naming a boundary
// honestly is cheaper than code nobody uses today:
//
//   - Lengths are counted in bytes, not characters. No string in the checked
//     set is affected; one with non-ASCII in it would be.
//   - An unsupported construct inside one branch of oneOf/anyOf can be hidden
//     by another branch that succeeds: the combinator is satisfied from the
//     outside while one of its alternatives was never understood. That is a
//     hole in this refusal scheme, not an oversight in a keyword.
//   - The register is complete for the schema of Codex CLI 0.155.1, checked by
//     walking every definition reachable from the types the shim answers.
//     A new Codex version needs that walk again: a keyword that appears in a
//     used type and is not in the register turns every run red, which is the
//     intended failure but still work to do. See docs/research.md.

// constraining keywords decide whether a value is allowed. Every one of them
// is implemented in walk.
var constraining = map[string]bool{
	"$ref": true, "allOf": true, "anyOf": true, "oneOf": true,
	"type": true, "enum": true,
	"properties": true, "required": true, "additionalProperties": true, "items": true,
	"minimum": true, "maximum": true, "minLength": true, "maxLength": true,
}

// annotation keywords carry documentation or defaults and cannot change
// whether a value is allowed.
//
//   - title, description, examples, $comment, deprecated: documentation.
//   - default: what a *writer* may use when omitting the field; it constrains
//     nothing about a value that is present.
//   - $schema: which dialect the file is written in, not a constraint on data.
//   - format: an annotation in JSON Schema, where validating it is optional.
//     In this bundle it records integer widths ("int64", "uint32") beside a
//     "type": "integer" that already constrains the value.
//   - readOnly, writeOnly: direction of use, not admissibility.
var annotation = map[string]bool{
	"title": true, "description": true, "examples": true, "$comment": true,
	"deprecated": true, "default": true, "$schema": true, "format": true,
	"readOnly": true, "writeOnly": true,
}

// schemaBundle is a set of named definitions from the generated bundles.
type schemaBundle struct {
	definitions map[string]json.RawMessage
}

type checker struct {
	bundle   *schemaBundle
	problems []string
	depth    int
}

// maxSchemaDepth stops a cycle in the schema from becoming a hang.
const maxSchemaDepth = 40

func (b *schemaBundle) check(typeName string, value any) []string {
	node, known := b.definitions[typeName]
	if !known {
		return []string{fmt.Sprintf("the schema has no type %q", typeName)}
	}
	c := &checker{bundle: b}
	c.walk(typeName, node, value)
	sort.Strings(c.problems)
	return c.problems
}

func (c *checker) fail(path, format string, args ...any) {
	c.problems = append(c.problems, path+": "+fmt.Sprintf(format, args...))
}

// walk applies every keyword of a node, rather than choosing one and
// returning: a node carrying both allOf and type has to satisfy both, and an
// earlier version silently dropped whatever came after the branch it took.
func (c *checker) walk(path string, raw json.RawMessage, value any) {
	c.depth++
	defer func() { c.depth-- }()
	if c.depth > maxSchemaDepth {
		c.fail(path, "schema nesting is deeper than this checker follows")
		return
	}
	var node map[string]json.RawMessage
	if err := json.Unmarshal(raw, &node); err != nil {
		c.fail(path, "unreadable schema node: %v", err)
		return
	}
	for keyword := range node {
		if !constraining[keyword] && !annotation[keyword] {
			c.fail(path, "the checker does not understand the schema keyword %q", keyword)
		}
	}
	if ref, ok := node["$ref"]; ok {
		c.walkRef(path, ref, value)
	}
	if variants, ok := node["allOf"]; ok {
		branches := c.list(path, "allOf", variants)
		for _, variant := range branches {
			c.walk(path, variant, value)
		}
	}
	if variants, ok := node["oneOf"]; ok {
		c.exactlyOne(path, variants, value)
	}
	if variants, ok := node["anyOf"]; ok {
		c.atLeastOne(path, variants, value)
	}
	if allowed, ok := node["enum"]; ok {
		c.enumeration(path, allowed, value)
	}
	c.byType(path, node, value)
	c.bounds(path, node, value)
}

func (c *checker) walkRef(path string, raw json.RawMessage, value any) {
	var ref string
	if json.Unmarshal(raw, &ref) != nil {
		c.fail(path, "unreadable $ref")
		return
	}
	name := strings.TrimPrefix(ref, "#/definitions/")
	node, known := c.bundle.definitions[name]
	if !known {
		// The v1 bundle points into the v2 one as "v2/Name"; both are loaded
		// under their plain names, so the last segment resolves it.
		if cut := strings.LastIndex(name, "/"); cut >= 0 {
			node, known = c.bundle.definitions[name[cut+1:]]
		}
	}
	if !known {
		c.fail(path, "the schema has no type %q", name)
		return
	}
	c.walk(path, node, value)
}

// exactlyOne is oneOf: one branch, not several. Matching two branches is as
// much a violation as matching none.
func (c *checker) exactlyOne(path string, raw json.RawMessage, value any) {
	variants := c.list(path, "oneOf", raw)
	if len(variants) == 0 {
		c.fail(path, "oneOf has no variants, so nothing can satisfy it")
		return
	}
	matched, reasons := c.trial(path, variants, value)
	switch matched {
	case 1:
		return
	case 0:
		c.fail(path, "no oneOf variant accepts this value (%s)", strings.Join(reasons, " | "))
	default:
		c.fail(path, "%d oneOf variants accept this value, which requires exactly one", matched)
	}
}

func (c *checker) atLeastOne(path string, raw json.RawMessage, value any) {
	variants := c.list(path, "anyOf", raw)
	if len(variants) == 0 {
		c.fail(path, "anyOf has no variants, so nothing can satisfy it")
		return
	}
	if matched, reasons := c.trial(path, variants, value); matched == 0 {
		c.fail(path, "no anyOf variant accepts this value (%s)", strings.Join(reasons, " | "))
	}
}

// trial reports how many variants accept the value, and why the others did not.
func (c *checker) trial(path string, variants []json.RawMessage, value any) (int, []string) {
	matched := 0
	var reasons []string
	for _, variant := range variants {
		attempt := &checker{bundle: c.bundle, depth: c.depth}
		attempt.walk(path, variant, value)
		if len(attempt.problems) == 0 {
			matched++
			continue
		}
		reasons = append(reasons, strings.Join(attempt.problems, ", "))
	}
	return matched, reasons
}

// enumeration compares by JSON value, types included: the string "1" and the
// number 1 are different values, and comparing their printed forms made them
// the same.
func (c *checker) enumeration(path string, raw json.RawMessage, value any) {
	var allowed []json.RawMessage
	if json.Unmarshal(raw, &allowed) != nil {
		c.fail(path, "unreadable enum")
		return
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		c.fail(path, "unencodable value: %v", err)
		return
	}
	for _, candidate := range allowed {
		if bytes.Equal(bytes.TrimSpace(candidate), encoded) {
			return
		}
	}
	c.fail(path, "%s is not one of the permitted values", encoded)
}

func (c *checker) byType(path string, node map[string]json.RawMessage, value any) {
	if kinds := c.kinds(path, node); len(kinds) > 0 && !matchesAny(kinds, value) {
		c.fail(path, "expected %s, got %s", strings.Join(kinds, " or "), kindOf(value))
		return
	}
	switch typed := value.(type) {
	case map[string]any:
		c.object(path, node, typed)
	case []any:
		if items, ok := node["items"]; ok {
			for i, element := range typed {
				c.walk(fmt.Sprintf("%s[%d]", path, i), items, element)
			}
		}
	}
}

// bounds applies the numeric and length limits the bundle uses.
func (c *checker) bounds(path string, node map[string]json.RawMessage, value any) {
	if raw, ok := node["minimum"]; ok {
		c.limit(path, raw, value, "minimum", func(v, limit float64) bool { return v >= limit })
	}
	if raw, ok := node["maximum"]; ok {
		c.limit(path, raw, value, "maximum", func(v, limit float64) bool { return v <= limit })
	}
	if text, ok := value.(string); ok {
		if raw, ok := node["minLength"]; ok {
			c.length(path, raw, len(text), "minLength", func(n, limit int) bool { return n >= limit })
		}
		if raw, ok := node["maxLength"]; ok {
			c.length(path, raw, len(text), "maxLength", func(n, limit int) bool { return n <= limit })
		}
	}
}

func (c *checker) limit(path string, raw json.RawMessage, value any, name string, ok func(float64, float64) bool) {
	number, isNumber := value.(float64)
	if !isNumber {
		return
	}
	var limit float64
	if json.Unmarshal(raw, &limit) != nil {
		c.fail(path, "unreadable %s", name)
		return
	}
	if !ok(number, limit) {
		c.fail(path, "%v violates %s %v", number, name, limit)
	}
}

func (c *checker) length(path string, raw json.RawMessage, length int, name string, ok func(int, int) bool) {
	var limit int
	if json.Unmarshal(raw, &limit) != nil {
		c.fail(path, "unreadable %s", name)
		return
	}
	if !ok(length, limit) {
		c.fail(path, "length %d violates %s %d", length, name, limit)
	}
}

func (c *checker) object(path string, node map[string]json.RawMessage, value map[string]any) {
	var properties map[string]json.RawMessage
	if raw, ok := node["properties"]; ok {
		if json.Unmarshal(raw, &properties) != nil {
			c.fail(path, "unreadable properties")
			return
		}
	}
	if raw, ok := node["required"]; ok {
		var required []string
		if json.Unmarshal(raw, &required) != nil {
			c.fail(path, "unreadable required")
			return
		}
		for _, name := range required {
			if _, present := value[name]; !present {
				c.fail(path, "required field %q is missing", name)
			}
		}
	}
	extra, extraSchema := c.extraPolicy(path, node)
	for name, field := range value {
		property, known := properties[name]
		if known {
			c.walk(path+"."+name, property, field)
			continue
		}
		// Reached whether or not the node listed any properties: a node that
		// forbids extras and names none forbids everything.
		switch {
		case extraSchema != nil:
			c.walk(path+"."+name, extraSchema, field)
		case !extra:
			c.fail(path, "%q is not a field of this type", name)
		}
	}
}

// extraPolicy reads additionalProperties: whether extras are allowed, and the
// schema they must satisfy when one is given.
func (c *checker) extraPolicy(path string, node map[string]json.RawMessage) (bool, json.RawMessage) {
	raw, ok := node["additionalProperties"]
	if !ok {
		// Absent means permitted, as JSON Schema defines it.
		return true, nil
	}
	var allowed bool
	if json.Unmarshal(raw, &allowed) == nil {
		return allowed, nil
	}
	var sub map[string]json.RawMessage
	if json.Unmarshal(raw, &sub) == nil {
		return true, raw
	}
	c.fail(path, "unreadable additionalProperties")
	return false, nil
}

func (c *checker) kinds(path string, node map[string]json.RawMessage) []string {
	raw, ok := node["type"]
	if !ok {
		return nil
	}
	var single string
	if json.Unmarshal(raw, &single) == nil {
		return []string{single}
	}
	var several []string
	if json.Unmarshal(raw, &several) == nil {
		return several
	}
	c.fail(path, "unreadable type")
	return nil
}

func (c *checker) list(path, keyword string, raw json.RawMessage) []json.RawMessage {
	var variants []json.RawMessage
	if json.Unmarshal(raw, &variants) != nil {
		c.fail(path, "unreadable %s", keyword)
		return nil
	}
	return variants
}
