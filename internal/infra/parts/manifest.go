// Package parts is rewake's parts and what restarting them takes
// (docs/install.md#the-parts-and-what-an-update-restarts): the manifest a
// build writes beside what it installs, with each part's digest, contracts and
// restart class; the digests themselves; and the plan, which compares the
// parts running with those installed. It writes nothing: the tools that
// install a build write the manifest, and nothing records the running set in
// the room.
package parts

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/praline-labs/rewake/internal/infra/build"
)

// FileName is the manifest's name, beside the installed rewake.
const FileName = "manifest.json"

// Format is the manifest's own version, raised with a change of its shape.
const Format = 1

// The restart classes: what it takes for a running part to become the
// installed one. A class the reader does not know counts as Room.
const (
	// Live: the part follows its files; nothing to do.
	Live = "live"
	// Shell: rewake restart shell, while main and the workers run on.
	Shell = "shell"
	// Sessions: each session takes the installed one when it starts.
	Sessions = "sessions"
	// Room: main ended and started again, its workers with it.
	Room = "room"
)

// The parts every build has. A harness rewake ships beside itself is a part
// too, named by its id.
const (
	Rewake    = "rewake"
	ShellPart = "shell"
	Face      = "face"
)

// The contracts between the parts, by the name a manifest gives them.
const (
	// HostContract is the host↔shell channel's version.
	HostContract = "host"
	// FaceContract is the face's contract with the shell.
	FaceContract = "face"
	// MirrorContract is the mirror's hello version, which the face reads.
	MirrorContract = "mirror"
)

// Manifest is the parts of one install.
type Manifest struct {
	Manifest int    `json:"manifest"`
	Parts    []Part `json:"parts"`
}

// Part is one part as installed, or as running. Digest is over what the part
// is made of, File over the file it ships as; either may be absent. Within
// names the part it is compiled into, whose file and class it shares. Speaks
// and Requires map a contract to its value; a running part whose Requires is
// nil said none, which the plan cannot take for compatible.
type Part struct {
	Name     string            `json:"name"`
	Class    string            `json:"class"`
	Digest   string            `json:"digest,omitempty"`
	File     string            `json:"file,omitempty"`
	Within   string            `json:"within,omitempty"`
	Speaks   map[string]string `json:"speaks,omitempty"`
	Requires map[string]string `json:"requires,omitempty"`
}

// ErrNone is an install with no manifest: a build by hand, or go run.
var ErrNone = errors.New("no manifest")

// Read is the manifest of the install in dir.
func Read(dir string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if errors.Is(err, os.ErrNotExist) {
		return Manifest{}, fmt.Errorf("%w beside %s", ErrNone, dir)
	}
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("the manifest %s does not read: %w", filepath.Join(dir, FileName), err)
	}
	if m.Manifest != Format {
		return Manifest{}, fmt.Errorf("the manifest %s is of format %d, this rewake reads %d", filepath.Join(dir, FileName), m.Manifest, Format)
	}
	return m, nil
}

// Encode is the manifest as its file holds it.
func Encode(m Manifest) []byte {
	m.Manifest = Format
	data, _ := json.MarshalIndent(m, "", "  ")
	return append(data, '\n')
}

// Find is the part named name.
func (m Manifest) Find(name string) (Part, bool) {
	for _, p := range m.Parts {
		if p.Name == name {
			return p, true
		}
	}
	return Part{}, false
}

// InstalledDir is the directory the running rewake was installed in, where
// its manifest is: for a room's own copy, the install its origin names
// (docs/rules/state.md#the-rooms-own-copy).
func InstalledDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return build.Origin(exe)
}
