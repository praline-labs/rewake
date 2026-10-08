package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The app-server schema, taken from a real Codex in the run: the installed
// one, or the version REWAKE_CODEX_VERSION names, run in a container (see
// harness_version_test.go).
//
// Three rounds of acceptance in a row found the shim answering in a shape the
// real server does not use, each time spotted by a person comparing against
// the source. Checking the shape mechanically closes that class; a copy of the
// schema in the repository would not, because it would go stale exactly the
// way the version pin did.
//
// What this proves and what it does not. It says a message has the fields its
// type requires and no fields that type does not have. It says nothing about
// *when* a message is sent, whose state it reflects, what happens on refusal,
// or whether a conversation was correlated with the request that asked for it
// — three of the five findings of the last round were of that kind. A shim
// that passes this is well-formed, not correct.
//
// It also steps outside the case's isolation on purpose: inside the case
// `codex` is the shim, so the schema comes from outside it. Nothing is
// executed inside the case and no credentials are touched — a file is
// generated and read.

// loadSchema generates the protocol schema and reads the two bundles. It
// answers three ways, because they mean three things: a bundle; a reason in
// absent when no Codex is there and none was asked for, which is unsupported;
// or an error when a Codex was there — installed or named — and did not
// produce a schema, which is red.
func loadSchema(c *Case, dir string) (bundle *schemaBundle, source schemaSource, err error) {
	if !suite.enabled {
		// The five ordinary checks start no harness: the schema needs the
		// switch, like every scenario.
		return nil, schemaSource{absent: "the workflow suite is switched off"}, nil
	}
	source = codexSchemaSource()
	if source.absent != "" || source.err != nil {
		return nil, source, source.err
	}
	if err := source.generate(c, dir); err != nil {
		return nil, source, err
	}
	bundle = &schemaBundle{definitions: map[string]json.RawMessage{}}
	for _, name := range []string{
		"codex_app_server_protocol.schemas.json",
		"codex_app_server_protocol.v2.schemas.json",
	} {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, source, fmt.Errorf("schema bundle %s is missing: %w", name, err)
		}
		var file struct {
			Definitions map[string]json.RawMessage `json:"definitions"`
		}
		if err := json.Unmarshal(raw, &file); err != nil {
			return nil, source, fmt.Errorf("schema bundle %s is unreadable: %w", name, err)
		}
		for typeName, definition := range file.Definitions {
			// The v2 bundle is read second and wins: it describes the API the
			// adapter speaks.
			bundle.definitions[typeName] = definition
		}
	}
	return bundle, source, nil
}

// asMessage turns a value the shim built into the map form check reads.
func asMessage(value any) (map[string]any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var message map[string]any
	if err := json.Unmarshal(raw, &message); err != nil {
		return nil, err
	}
	return message, nil
}

// describe joins problems for a verdict message.
func describe(problems []string) string { return strings.Join(problems, "; ") }
