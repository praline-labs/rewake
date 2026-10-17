package internal

import (
	"fmt"
	"slices"
	"strings"
)

// The layers of 2.0 (docs/v2/design.md#the-layout). An adapter's layer carries
// its name, so that one adapter importing another is told apart from an adapter
// importing its own parts.
const (
	infra   = "infra"
	core    = "core"
	tool    = "tool"
	api     = "adapter"
	catalog = "adapter/catalog"
	host    = "host"
	cli     = "cli"
	command = "cmd"

	// external is any package of another module. The design's table names one
	// layer's limit on them, infra's "the standard library only"; for the others it
	// is silent, and each dependency is its own decision (AGENTS.md, Code).
	external = "external"
)

// mayImport is the import rule's table. A layer may always import itself, and
// an adapter its own parts; infra may import infra (correction 1 of
// docs/v2/stage3.md, agreed in review: state imports boottime).
var mayImport = map[string][]string{
	infra: {},
	core:  {infra, external},
	tool:  {core, infra, external},
	api:   {core, infra, external},
	// Every adapter alike: the API, the tools and the core, never another adapter.
	"adapter/*": {api, tool, core, infra, external},
	catalog:     {api, "adapter/*", external},
	host:        {api, tool, core, infra, external},
	cli:         {host, api, tool, core, infra, external},
	command:     {cli, catalog, external},
}

// confined names the modules that one package alone may import, by path
// prefix: a library taken as an exception to the dependency rule stays behind
// the package it was taken for, so no other code comes to lean on it
// (docs/v2/stage4-space.md#the-terminal-library).
var confined = map[string]string{
	"github.com/charmbracelet/": "internal/host/terminal",
}

// confinedTo answers which package alone may import path, when one is named.
func confinedTo(path string) (string, bool) {
	for prefix, owner := range confined {
		if strings.HasPrefix(path, prefix) {
			return owner, true
		}
	}
	return "", false
}

func isAdapter(layer string) bool {
	return layer != catalog && strings.HasPrefix(layer, api+"/")
}

func underAdapter(layer string) bool {
	return layer == api || strings.HasPrefix(layer, api+"/")
}

// allowed answers whether a package of one layer may import one of another.
func allowed(from, to string) bool {
	if from == to {
		return true
	}
	for _, may := range mayImport[layerKey(from)] {
		if may == to || may == "adapter/*" && isAdapter(to) {
			return true
		}
	}
	return false
}

func layerKey(layer string) string {
	if isAdapter(layer) {
		return "adapter/*"
	}
	return layer
}

// layerOfPath is the layer a 2.0 path names.
func layerOfPath(path string) (string, bool) {
	switch {
	case path == "cmd/rewake":
		return command, true
	case path == "internal/adapter":
		return api, true
	case path == "internal/adapter/catalog":
		return catalog, true
	case strings.HasPrefix(path, "internal/adapter/"):
		name, _, _ := strings.Cut(strings.TrimPrefix(path, "internal/adapter/"), "/")
		return api + "/" + name, true
	}
	for _, layer := range []string{infra, core, tool, host, cli} {
		if path == "internal/"+layer || strings.HasPrefix(path, "internal/"+layer+"/") {
			return layer, true
		}
	}
	return "", false
}

// goFile is one Go file of a build variant, by its path relative to the module
// root, with the import paths it names.
type goFile struct {
	name    string
	imports []string
}

type goPackage struct {
	path  string
	files []goFile
}

// variant is the module's graph under one build, with the packages the go
// command reports as the standard library's, which every layer may use.
type variant struct {
	build    build
	packages []goPackage
	standard map[string]bool
}

// edge is one forbidden import, found in a file under some variants.
type edge struct {
	file, from, to     string
	fromLayer, toLayer string
	builds             []build
	// owner is set for a confined module's import: the package that alone may
	// import it.
	owner string
}

func (e edge) String() string {
	if e.owner != "" {
		return fmt.Sprintf("%s: %s imports %s, which only %s may import (docs/rules/layout.md#the-import-rule)%s",
			e.file, e.from, e.to, e.owner, underBuilds(e.builds))
	}
	return fmt.Sprintf("%s: %s (%s) imports %s (%s), which the import rule forbids (docs/v2/design.md#the-import-rule)%s",
		e.file, e.from, e.fromLayer, e.to, e.toLayer, underBuilds(e.builds))
}

// underBuilds says which builds an edge exists in when the plain one is not
// among them, since such an edge is fixed in a file a plain build never reads.
func underBuilds(builds []build) string {
	if slices.ContainsFunc(builds, func(b build) bool { return len(b.flags()) == 0 }) {
		return ""
	}
	var names []string
	for _, b := range builds {
		names = append(names, b.String())
	}
	return " (only under " + strings.Join(names, "; ") + ")"
}

// judgeImports checks every edge of every variant against the import rule; the
// placement errors are packages at no 2.0 path.
func judgeImports(variants []variant) ([]edge, []string) {
	var found []edge
	var unplaced []string
	for _, v := range variants {
		for _, pkg := range v.packages {
			from, ok := layerOfPath(pkg.path)
			if !ok {
				unplaced = appendNew(unplaced, unplacedProblem(pkg.path))
				continue
			}
			for _, file := range pkg.files {
				for _, path := range file.imports {
					if v.standard[path] {
						continue
					}
					imported, to := path, external
					if within, ok := strings.CutPrefix(path, module+"/"); ok {
						imported = within
						if to, ok = layerOfPath(imported); !ok {
							unplaced = appendNew(unplaced, unplacedProblem(imported))
							continue
						}
					}
					if !allowed(from, to) {
						found = addEdge(found, edge{file: file.name, from: pkg.path, to: imported, fromLayer: from, toLayer: to}, v.build)
					} else if owner, ok := confinedTo(path); ok && pkg.path != owner {
						found = addEdge(found, edge{file: file.name, from: pkg.path, to: imported, owner: owner}, v.build)
					}
				}
			}
		}
	}
	return found, unplaced
}

func unplacedProblem(path string) string {
	return fmt.Sprintf("%s is at no 2.0 path: move it under a layer of docs/v2/design.md#the-layout", path)
}

func addEdge(found []edge, e edge, b build) []edge {
	for i := range found {
		if found[i].file == e.file && found[i].from == e.from && found[i].to == e.to {
			found[i].builds = append(found[i].builds, b)
			return found
		}
	}
	e.builds = []build{b}
	return append(found, e)
}

func appendNew(list []string, item string) []string {
	if slices.Contains(list, item) {
		return list
	}
	return append(list, item)
}
