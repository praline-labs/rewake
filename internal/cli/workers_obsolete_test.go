package cli

// The catalog as workers shows it when a file still carries the old worker
// type: whole-template precedence, and a current setting that is malformed
// beside it is refused rather than screened.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// obsoleteRoom is a private home and working directory with the two alias
// files, each written only when its text is given.
func obsoleteRoom(t *testing.T, user, project string) {
	t.Helper()
	home, work := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(work)
	for path, text := range map[string]string{
		filepath.Join(home, ".config", "rewake", "aliases.toml"): user,
		filepath.Join(work, ".rewake", "aliases.toml"):           project,
	} {
		if text == "" {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// workersJSON is the listing a caller reads, with its stderr.
func workersJSON(t *testing.T) (workersListing, string) {
	t.Helper()
	code, out, errOut := runOn(testCatalog(), "workers", "--json")
	if code != ExitOK {
		t.Fatalf("workers --json exited %d: %s", code, errOut)
	}
	var listing workersListing
	if err := json.Unmarshal([]byte(out), &listing); err != nil {
		t.Fatalf("workers --json: %v\n%s", err, out)
	}
	return listing, errOut
}

// A template defined in both files follows whole-template precedence: the
// layer that defines it whole decides, so an obsolete definition never fails
// the listing nor leaks into the other layer.
func TestWorkersFollowsWholeTemplatePrecedence(t *testing.T) {
	const kept = "[alias.keep]\nharness = \"agent\"\n"
	for name, tc := range map[string]struct {
		user, project      string
		names, description string
		notice             string
	}{
		"obsolete user, valid project": {
			"[alias.w]\nharness = \"agent\"\ntype = \"code\"\n" + kept,
			"[alias.w]\nharness = \"agent\"\ndescription = \"project w\"\n",
			"keep,w", "project w", "",
		},
		"valid user, obsolete project": {
			"[alias.w]\nharness = \"agent\"\ndescription = \"user w\"\n" + kept,
			"[alias.w]\nharness = \"agent\"\ntype = \"code\"\n",
			"keep", "", "obsolete type field",
		},
		"obsolete user only": {
			"[alias.w]\nharness = \"agent\"\ntype = \"code\"\n" + kept,
			"",
			"keep", "", "obsolete type field",
		},
		"obsolete project only": {
			kept,
			"[alias.w]\nharness = \"agent\"\ntype = \"code\"\n",
			"keep", "", "obsolete type field",
		},
	} {
		obsoleteRoom(t, tc.user, tc.project)
		listing, errOut := workersJSON(t)
		var names []string
		for _, worker := range listing.Workers {
			names = append(names, worker.Name)
			if worker.Name == "w" && tc.description != "" && worker.Description != tc.description {
				t.Errorf("%s: the definition in force is %q, want %q", name, worker.Description, tc.description)
			}
		}
		if got := strings.Join(names, ","); got != tc.names {
			t.Errorf("%s: workers %q, want %q", name, got, tc.names)
		}
		if tc.notice == "" {
			if strings.Contains(errOut, "obsolete type field") {
				t.Errorf("%s: a notice for a definition not in force: %s", name, errOut)
			}
			continue
		}
		if !strings.Contains(errOut, tc.notice) {
			t.Errorf("%s: the notice: %s", name, errOut)
		}
	}
}

// A template the obsolete type left out is still held to the current schema:
// workers refuses a malformed setting beside it instead of screening it.
func TestWorkersRefusesAMalformedSettingBesideAnObsoleteType(t *testing.T) {
	for name, tc := range map[string]struct{ file, want string }{
		"start_mode plan":         {"[alias.w]\nharness = \"agent\"\ntype = \"code\"\n[alias.w.pi]\nstart_mode = \"plan\"\n", "plan"},
		"unknown harness":         {"[alias.w]\nharness = \"nothing\"\ntype = \"code\"\n", "which is not a harness"},
		"settings without a flag": {"[alias.w]\nharness = \"agent\"\ntype = \"code\"\n[alias.w.launch]\nmodel = \"m\"\n", "no launch settings"},
	} {
		obsoleteRoom(t, "", tc.file)
		code, _, errOut := runOn(testCatalog(), "workers", "--json")
		if code != ExitUsage || !strings.Contains(errOut, tc.want) {
			t.Errorf("%s: exited %d, want exit %d saying %q: %s", name, code, ExitUsage, tc.want, errOut)
		}
	}
}
