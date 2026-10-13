package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// moduleRoot is where go test runs this package from, two levels down.
const moduleRoot = "../.."

// The checked-in files are what the schema generates today: an edit of the
// schema without a run of the generator, or of a generated file by hand,
// fails here rather than as a mod and an endpoint that disagree at run time.
func TestTheCheckedInFilesMatchTheSchema(t *testing.T) {
	files, err := generateAt(moduleRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		checkedIn, err := os.ReadFile(filepath.Join(moduleRoot, file.path))
		if err != nil {
			t.Errorf("%s: %v; run go run ./tools/protogen from the module root", file.path, err)
			continue
		}
		if !bytes.Equal(checkedIn, file.content) {
			t.Errorf("%s differs from a fresh generation from %s: run go run ./tools/protogen from the module root", file.path, schemaPath)
		}
	}
}

// A run writes only what differs, so a run on an unchanged schema leaves the
// files and their times alone.
func TestARunRewritesOnlyWhatDiffers(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{schemaPath, tsPath, goPath} {
		text, err := os.ReadFile(filepath.Join(moduleRoot, path))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, path), text, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out strings.Builder
	if err := run(root, &out); err != nil || out.Len() != 0 {
		t.Fatalf("an unchanged schema: %q, %v", out.String(), err)
	}
	if err := os.WriteFile(filepath.Join(root, tsPath), []byte("edited by hand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(root, &out); err != nil || out.String() != "wrote "+tsPath+"\n" {
		t.Fatalf("one file edited by hand: %q, %v", out.String(), err)
	}
}

// small is a schema of one endpoint whose request is def, so a case changes
// one definition and sees one refusal.
func small(def string) string {
	return `{"title": "t", "description": "d",
	"x-bounds": {"maxBodyBytes": {"value": 1, "description": "is a bound."}},
	"x-endpoints": [{"path": "one", "description": "is one.", "request": {"$ref": "#/$defs/Asked"}, "answer": {"$ref": "#/$defs/Asked"}}],
	"$defs": {"Asked": ` + def + `, "Kind": {"type": "string", "description": "is a kind.", "enum": ["one", "two"]}}}`
}

const closed = `"required": [], "additionalProperties": false`

// The generator reads the subset the schema uses and refuses the rest by
// name, so the schema cannot state a constraint neither side is generated
// from, nor a shape the two sides would name differently.
func TestTheGeneratorRefusesWhatItDoesNotRead(t *testing.T) {
	for name, c := range map[string]struct{ def, want string }{
		"a validation word":  {`{"type": "object", "description": "x", "properties": {"n": {"type": "integer", "minimum": 0, "description": "n."}}, ` + closed + `}`, `unknown field "minimum"`},
		"an object in place": {`{"type": "object", "description": "x", "properties": {"o": {"type": "object", "description": "o.", "properties": {}, ` + closed + `}}, ` + closed + `}`, "a definition of its own"},
		"an enum in place":   {`{"type": "object", "description": "x", "properties": {"e": {"type": "string", "enum": ["a"], "description": "e."}}, ` + closed + `}`, "a definition of its own"},
		"an open object":     {`{"type": "object", "description": "x", "properties": {}, "required": [], "additionalProperties": true}`, "additionalProperties to false"},
		"no description":     {`{"type": "object", "description": "x", "properties": {"s": {"type": "string"}}, ` + closed + `}`, "s: a property has a description"},
		"a stray required":   {`{"type": "object", "description": "x", "properties": {}, "required": ["s"], "additionalProperties": false}`, `required names "s"`},
		"a missing ref":      {`{"type": "object", "description": "x", "properties": {"r": {"$ref": "#/$defs/Gone", "description": "r."}}, ` + closed + `}`, `"Gone", which is no definition`},
		"a foreign ref":      {`{"type": "object", "description": "x", "properties": {"r": {"$ref": "other.json#/X", "description": "r."}}, ` + closed + `}`, "a reference names #/$defs/"},
		"a name twice":       {`{"type": "object", "description": "x", "properties": {"s": {"type": "string", "description": "s."}, "s": {"type": "string", "description": "s."}}, ` + closed + `}`, `names "s" twice`},
		"a number":           {`{"type": "object", "description": "x", "properties": {"f": {"type": "number", "description": "f."}}, ` + closed + `}`, "a string, an integer, a boolean"},
		"an enum as request": {`{"type": "string", "description": "x", "enum": ["a"]}`, "reference object definitions"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseSchema([]byte(small(c.def)))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %v, want a refusal saying %q", err, c.want)
			}
		})
	}
}

// An optional field stays apart from its zero on both sides: a palette index
// of 0 is black, not an absent color.
func TestAnOptionalFieldStaysApartFromItsZero(t *testing.T) {
	s, err := parseSchema([]byte(small(`{"type": "object", "description": "x", "properties": {
		"index": {"type": "integer", "description": "is optional."},
		"cols": {"type": "integer", "description": "is required."},
		"kinds": {"type": "array", "items": {"$ref": "#/$defs/Kind"}, "description": "is optional."},
		"kind": {"$ref": "#/$defs/Kind", "description": "is optional."}},
		"required": ["cols"], "additionalProperties": false}`)))
	if err != nil {
		t.Fatal(err)
	}
	source, err := emitGo(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Index *int `json:\"index,omitempty\"`",
		"Cols int `json:\"cols\"`",
		"Kinds []Kind `json:\"kinds,omitempty\"`",
		"Kind *Kind `json:\"kind,omitempty\"`",
		`KindOne Kind = "one"`,
	} {
		if !strings.Contains(string(source), want) {
			t.Errorf("the Go side lacks %q:\n%s", want, source)
		}
	}
	ts := string(emitTS(s))
	for _, want := range []string{"  index?: number\n", "  cols: number\n", "  kinds?: Kind[]\n", "  kind?: Kind\n", `export type Kind = "one" | "two"`} {
		if !strings.Contains(ts, want) {
			t.Errorf("the TypeScript side lacks %q:\n%s", want, ts)
		}
	}
}
