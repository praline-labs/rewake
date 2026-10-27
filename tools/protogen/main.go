// Command protogen writes the types of the mod's protocol from its one JSON
// schema: TypeScript for the mod, Go for its endpoint
// (docs/claude.md#the-protocol). Run it from the module root after
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

// target is a schema and the two files written from it, from the module
// root, the Go one in the package named.
type target struct {
	schema, ts, goFile, goPackage string
}

// The schema of the mod's protocol and the one of the room view the shell
// reads (docs/shell.md), with the files each generates.
const (
	schemaPath = "internal/adapter/claude/mod/protocol.schema.json"
	tsPath     = "internal/adapter/claude/mod/protocol.gen.ts"
	goPath     = "internal/adapter/claude/modhost/protocol_gen.go"
)

var targets = []target{
	{schemaPath, tsPath, goPath, "modhost"},
	{"internal/cli/room_view.schema.json", "shell/src/room_view.gen.ts", "internal/cli/room_view_gen.go", "cli"},
}

func main() {
	if err := run(".", os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "protogen:", err)
		os.Exit(1)
	}
}

// run regenerates the files of every target under root and names those it changed.
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

// generateAt reads each schema under root and returns what its files should
// hold, writing nothing: the test compares it with the checked-in files.
func generateAt(root string) ([]generated, error) {
	var files []generated
	for _, t := range targets {
		text, err := os.ReadFile(filepath.Join(root, t.schema))
		if err != nil {
			return nil, err
		}
		schema, err := parseSchema(text)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", t.schema, err)
		}
		schema.source, schema.goPackage = filepath.Base(t.schema), t.goPackage
		goSource, err := emitGo(schema)
		if err != nil {
			return nil, err
		}
		files = append(files, generated{path: t.ts, content: emitTS(schema)}, generated{path: t.goFile, content: goSource})
	}
	return files, nil
}
