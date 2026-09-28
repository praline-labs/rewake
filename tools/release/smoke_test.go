package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A build is found by the alias directory it sits in, nested under the entry
// or hoisted beside it, and the entry itself is not taken for one.
func TestInstalledBuildsAreFoundWhereverNpmPutThem(t *testing.T) {
	modules := t.TempDir()
	write := func(dir, version string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(modules, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		content := `{"name":"` + packageName + `","version":"` + version + `"}`
		if err := os.WriteFile(filepath.Join(modules, dir, "package.json"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("@praline-labs/rewake", "1.0.2")
	write("@praline-labs/rewake/node_modules/@praline-labs/rewake-linux-x64", "1.0.2-linux-x64")
	write("@praline-labs/rewake-linux-arm64", "1.0.2-linux-arm64")
	write("other/node_modules/unrelated", "1.0.0")

	builds, err := installedBuilds(modules)
	want := map[string]string{scope + "/rewake-linux-x64": "1.0.2-linux-x64", scope + "/rewake-linux-arm64": "1.0.2-linux-arm64"}
	if err != nil || !maps.Equal(builds, want) {
		t.Errorf("builds = %v, %v; want %v", builds, err, want)
	}
}

// The local registry answers the scoped name as npm asks for it, with its slash
// escaped, lists every version under its tag with the tarball's integrity, and
// serves the tarballs.
func TestTheLocalRegistryServesTheRelease(t *testing.T) {
	packs := 0
	f := &fake{answer: func(line string) result {
		if !strings.HasPrefix(line, "npm pack") {
			return result{}
		}
		fields := strings.Fields(line)
		packs++
		file := filepath.Join(fields[len(fields)-1], fmt.Sprintf("upload-%d.tgz", packs))
		if err := os.WriteFile(file, []byte(file), 0o644); err != nil {
			return result{code: 1, stderr: err.Error()}
		}
		return result{stdout: `[{"filename":"` + filepath.Base(file) + `"}]`}
	}}
	g, _ := newGate(t, call{version: "1.0.2"}, f)
	for _, p := range packages {
		if err := os.MkdirAll(g.dist(p), 0o755); err != nil {
			t.Fatal(err)
		}
		content := `{"name":"` + packageName + `","version":"` + p.version(g.version) + `"}`
		if err := os.WriteFile(filepath.Join(g.dist(p), "package.json"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	registry, err := g.serveRelease(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer registry.close()

	get := func(path string) []byte {
		t.Helper()
		response, err := http.Get(registry.url + path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = response.Body.Close() }()
		body, err := io.ReadAll(response.Body)
		if err != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: %d %v", path, response.StatusCode, err)
		}
		return body
	}
	var document struct {
		Tags     map[string]string `json:"dist-tags"`
		Versions map[string]struct {
			Dist struct {
				Tarball   string `json:"tarball"`
				Integrity string `json:"integrity"`
			} `json:"dist"`
		} `json:"versions"`
	}
	if err := json.Unmarshal(get("@praline-labs%2frewake"), &document); err != nil {
		t.Fatal(err)
	}
	wantTags := map[string]string{"latest": "1.0.2", "linux-x64": "1.0.2-linux-x64", "linux-arm64": "1.0.2-linux-arm64"}
	if !maps.Equal(document.Tags, wantTags) {
		t.Errorf("dist-tags %v, want %v", document.Tags, wantTags)
	}
	for _, version := range wantTags {
		dist := document.Versions[version].Dist
		if !strings.HasPrefix(dist.Integrity, "sha512-") || !strings.HasPrefix(dist.Tarball, registry.url) {
			t.Errorf("%s: dist %+v", version, dist)
			continue
		}
		if body := get(strings.TrimPrefix(dist.Tarball, registry.url)); !strings.HasSuffix(string(body), ".tgz") {
			t.Errorf("%s: tarball %q", version, body)
		}
	}
}
