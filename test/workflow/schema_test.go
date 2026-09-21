package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// The app-server schema, taken from the installed Codex in the run.
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
// It also steps outside the case's isolation on purpose: the schema comes from
// the Codex installed on the machine, because inside the case `codex` is the
// shim. Nothing is executed inside the case and no credentials are touched —
// a file is generated and read. Without an installed Codex there is no schema,
// and the check skips with a reason while the scenario itself carries on: the
// scenario needs the shim, not Codex.

// loadSchema generates the protocol schema with the installed Codex and reads
// the two bundles. It returns a reason instead of an error when Codex is
// simply not there, so the caller can skip rather than fail.
func loadSchema(c *Case, dir string) (*schemaBundle, string) {
	codex, err := exec.LookPath("codex")
	if err != nil {
		return nil, "no codex on PATH, so no schema to check against"
	}
	// --experimental on purpose: fields behind that flag are exactly the ones
	// the adapter uses — canAcceptDirectInput and runtimeWorkspaceRoots are
	// both experimental — and a schema generated without it describes a
	// narrower protocol than the one being spoken. Without the flag the check
	// would report the shim's correct canAcceptDirectInput as an invented
	// field.
	generate := exec.Command(codex, "app-server", "generate-json-schema", "--experimental", "--out", dir)
	// Deliberately the ambient environment: this is the installed Codex, not
	// anything inside the case.
	generate.Env = os.Environ()
	if out, err := c.Output(generate); err != nil {
		return nil, fmt.Sprintf("codex could not produce a schema: %v: %s", err, out)
	}
	bundle := &schemaBundle{definitions: map[string]json.RawMessage{}}
	for _, name := range []string{
		"codex_app_server_protocol.schemas.json",
		"codex_app_server_protocol.v2.schemas.json",
	} {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Sprintf("schema bundle %s is missing: %v", name, err)
		}
		var file struct {
			Definitions map[string]json.RawMessage `json:"definitions"`
		}
		if err := json.Unmarshal(raw, &file); err != nil {
			return nil, fmt.Sprintf("schema bundle %s is unreadable: %v", name, err)
		}
		for typeName, definition := range file.Definitions {
			// The v2 bundle is read second and wins: it describes the API the
			// adapter speaks.
			bundle.definitions[typeName] = definition
		}
	}
	return bundle, ""
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
