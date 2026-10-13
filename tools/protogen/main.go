// Command protogen writes the types of the mod's protocol from its one JSON
// schema: TypeScript for the mod, Go for its endpoint
// (docs/v2/stage4-space.md#2-the-protocol). Run it from the module root after
// editing the schema:
//
//	go run ./tools/protogen
//
// Why a generator of rewake's own: the schema uses a handful of JSON Schema's
// words, and a general implementation would be a dependency that reads far
// more than this file ever says. It reads exactly the subset the schema uses —
// objects, strings, integers, booleans, arrays, string enums and references —
// and refuses any other word, so the schema cannot say something neither side
// is generated from.
//
// Why it lives in tools/: it ships with nobody, and the layers of internal/
// may not import it; its test, which holds the checked-in files to a fresh
// generation, runs with the five checks.
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// The schema and the files written from it, from the module root.
const (
	schemaPath = "internal/adapter/claude/mod/protocol.schema.json"
	tsPath     = "internal/adapter/claude/mod/protocol.gen.ts"
	goPath     = "internal/adapter/claude/modhost/protocol_gen.go"
	goPackage  = "modhost"
)

func main() {
	if err := run(".", os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "protogen:", err)
		os.Exit(1)
	}
}

// run regenerates both files under root and names those it changed.
func run(root string, out io.Writer) error {
	files, err := generateAt(root)
	if err != nil {
		return err
	}
	for _, file := range files {
		path := filepath.Join(root, file.path)
		old, err := os.ReadFile(path)
		if err == nil && bytes.Equal(old, file.content) {
			continue
		}
		if err := os.WriteFile(path, file.content, 0o644); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, "wrote", file.path)
	}
	return nil
}

type generated struct {
	path    string
	content []byte
}

// generateAt reads the schema under root and returns what both files should
// hold, writing nothing: the test compares it with the checked-in files.
func generateAt(root string) ([]generated, error) {
	text, err := os.ReadFile(filepath.Join(root, schemaPath))
	if err != nil {
		return nil, err
	}
	schema, err := parseSchema(text)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", schemaPath, err)
	}
	goSource, err := emitGo(schema)
	if err != nil {
		return nil, err
	}
	return []generated{
		{path: tsPath, content: emitTS(schema)},
		{path: goPath, content: goSource},
	}, nil
}
