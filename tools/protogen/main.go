// Command protogen writes the types of the mod's protocol from its one JSON
// schema: TypeScript for the mod, Go for its endpoint
// (docs/claude.md#the-protocol); and it copies the TypeScript modules every
// adapter's harness side shares — the mirror's feed (docs/face.md, "The
// feed") and the session tools (docs/code.md) — into each adapter that
// embeds them. Run it from the module root after editing a schema or a shared
// module:
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

// The schema of the mod's protocol, the one of the room view the shell reads
// (docs/shell.md) and the mirror's (docs/face.md), with the files each
// generates.
const (
	schemaPath = "internal/adapter/claude/mod/protocol.schema.json"
	tsPath     = "internal/adapter/claude/mod/protocol.gen.ts"
	goPath     = "internal/adapter/claude/modhost/protocol_gen.go"
)

var targets = []target{
	{schemaPath, tsPath, goPath, "modhost"},
	{"internal/cli/room_view.schema.json", "shell/src/room_view.gen.ts", "internal/cli/room_view_gen.go", "cli"},
	{"internal/core/mirror/mirror.schema.json", "face/src/mirror.gen.ts", "internal/core/mirror/mirror_gen.go", "mirror"},
}

// shared is a TypeScript module both adapters' harness sides use, and the
// copies written from it. Each adapter embeds files of its own folder alone,
// and the mod's host imports no file outside the mod, so each folder holds a
// copy, marked as one and written from the source.
type shared struct {
	source string
	copies []string
}

// The mirror's feed, which both send through, and the session tools both
// offer.
var shareds = []shared{
	{"internal/core/mirror/feed.ts", []string{"internal/adapter/pi/feed.ts", "internal/adapter/claude/mod/hooks/feed.ts"}},
	{"internal/core/sessiontools/sessiontools.ts", []string{"internal/adapter/pi/sessiontools.ts", "internal/adapter/claude/mod/hooks/sessiontools.ts"}},
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
	for _, module := range shareds {
		text, err := os.ReadFile(filepath.Join(root, module.source))
		if err != nil {
			return nil, err
		}
		for _, path := range module.copies {
			files = append(files, generated{path: path, content: copyOf(module.source, text)})
		}
	}
	return files, nil
}

// copyOf is a shared module as a copy holds it, under a line naming its source.
func copyOf(source string, text []byte) []byte {
	header := "// A copy of " + source + ", written by go run ./tools/protogen: edit that file.\n"
	return append([]byte(header), text...)
}
