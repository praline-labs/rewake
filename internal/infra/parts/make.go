package parts

import (
	"encoding/json"
	"fmt"
	"slices"
)

// Made is what a tool that installs rewake knows of the parts it put in
// place, from which Make writes their manifest: the digests it took, and what
// the built rewake and shell say of their own contracts, so no tool reads a
// contract from the sources.
type Made struct {
	// Rewake has its Digest and File.
	Rewake Part
	// Version is what rewake --version --json printed.
	Version []byte
	// Shell has its Digest, and its File when it is compiled.
	Shell Part
	// Contracts is what rewake-shell --contracts printed.
	Contracts []byte
	// Face has its Digest, and Within when it is compiled into the shell.
	Face Part
	// Harnesses are the harnesses installed beside rewake, by HarnessPart.
	Harnesses []Part
}

// HarnessPart is a harness rewake ships beside itself, at the version an
// npm package installed: a version of a package never changes, so it is the
// digest, and each new session takes it.
func HarnessPart(name, pkg, version string) Part {
	return Part{Name: name, Class: Sessions, Digest: "npm:" + pkg + "@" + version, Speaks: map[string]string{name: version}}
}

// Make is the manifest of what m installed.
func Make(m Made) (Manifest, error) {
	var version struct {
		Contracts *struct{ Speaks, Requires map[string]string }
	}
	if err := json.Unmarshal(m.Version, &version); err != nil || version.Contracts == nil {
		return Manifest{}, fmt.Errorf("rewake --version --json names no contracts: %q", m.Version)
	}
	var shell struct {
		Shell *struct{ Requires map[string]string }
		Face  *struct{ Speaks, Requires map[string]string }
	}
	if err := json.Unmarshal(m.Contracts, &shell); err != nil || shell.Shell == nil || shell.Face == nil {
		return Manifest{}, fmt.Errorf("rewake-shell --contracts names no contracts: %q", m.Contracts)
	}
	rewake := m.Rewake
	rewake.Name, rewake.Class = Rewake, Room
	rewake.Speaks, rewake.Requires = version.Contracts.Speaks, version.Contracts.Requires
	sh := m.Shell
	sh.Name, sh.Class, sh.Requires = ShellPart, Shell, shell.Shell.Requires
	face := m.Face
	face.Name, face.Class = Face, Live
	if face.Within != "" {
		face.Class = Shell
	}
	face.Speaks, face.Requires = shell.Face.Speaks, shell.Face.Requires
	out := Manifest{Manifest: Format, Parts: []Part{rewake, sh, face}}
	harnesses := slices.Clone(m.Harnesses)
	slices.SortFunc(harnesses, func(a, b Part) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})
	out.Parts = append(out.Parts, harnesses...)
	return out, nil
}
