package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// shellMarker is what the installed shell's main prints; the shell draws it
// on its own screen, and the gate looks for it in what the shell wrote.
const shellMarker = "rewake-release-shell-check"

// installedShell requires npm to have put the compiled shell beside the
// installed binary, where rewake looks for it (internal/cli/shell.go), and
// that shell to start and draw main's output. Without a terminal it draws to
// its standard output all the same. main waits a second before it ends: the
// shell ends with main, and output of a main gone before its first frame is
// never drawn.
func (g *gate) installedShell(ctx context.Context, modules string, p pkg, scratch string) {
	ref := p.ref(g.version)
	for _, dir := range []string{filepath.Join(modules, packageName, "node_modules", p.alias()), filepath.Join(modules, p.alias())} {
		if _, err := os.Stat(filepath.Join(dir, "bin", "rewake")); err != nil {
			continue
		}
		shell := filepath.Join(dir, "bin", "rewake-shell")
		r := g.run(ctx, runLimit, scratch, shell, "--room", "release", "--", "/bin/sh", "-c", "echo "+shellMarker+"; sleep 1")
		if r.ok() && strings.Contains(r.stdout, shellMarker) {
			g.pass("%s: the installed shell beside bin/rewake starts and draws main's output", ref)
		} else {
			g.fail("%s: the installed shell %s did not draw main's output: %s", ref, shell, r.why())
		}
		return
	}
	g.fail("%s: npm installed no bin/rewake under %s, beside which the shell would be", ref, modules)
}
