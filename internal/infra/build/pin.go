package build

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// The room's own copy of its build (docs/rules/state.md#the-rooms-own-copy):
// an upgrade replaces the file at the install path under running sessions, so
// every session the room starts runs rewake from here, and the room keeps
// working on its build until a launch of another one switches it.
const (
	pinDir = "bin"
	// pinName is the copy's name, the one a session's PATH finds.
	pinName = "rewake"
	// OriginName is the file beside the copy naming the directory its image
	// was installed in, where what rewake ships beside itself is found.
	OriginName = "origin"
)

// Pinned is the path of room's own copy of its build.
func Pinned(room string) string { return filepath.Join(room, pinDir, pinName) }

// Executable is the rewake the room's sessions run: its own copy, or this
// process's executable when the launch could not place one.
func (l *Lease) Executable() string {
	if l == nil {
		return ""
	}
	return l.executable
}

// pin makes the room's copy this build's when it is missing or another's,
// under the lease the launch just took, before any other lock of the room. It
// returns the path the sessions get and a note when there is none; a failed
// pin does not stop the launch.
func pin(room, own string) (string, string) {
	self, err := os.Executable()
	if err == nil {
		self, err = filepath.EvalSymlinks(self)
	}
	if err != nil {
		return "", fmt.Sprintf("Rewake: room %s keeps no copy of its build (rewake's own path is unknown: %v); an upgrade of rewake stops its commands until main is restarted.", filepath.Base(room), err)
	}
	path := Pinned(room)
	if id, err := FileID(path); err == nil && id == own {
		return path, ""
	}
	if err := place(path, self, own); err != nil {
		return self, fmt.Sprintf("Rewake: room %s keeps no copy of its build (%v); an upgrade of rewake stops its commands until main is restarted.", filepath.Base(room), err)
	}
	return path, ""
}

// place writes the copy, its origin first: a copy is never without one.
func place(path, self, own string) error {
	dir := filepath.Dir(path)
	if err := state.EnsureSubdir(dir); err != nil {
		return err
	}
	// A copy launching from the room keeps the origin it was placed with.
	if filepath.Dir(self) != dir {
		if err := state.WriteAtomic(filepath.Join(dir, OriginName), []byte(filepath.Dir(self)+"\n")); err != nil {
			return err
		}
	}
	return state.PlaceExecutable(path, self, image, func(temp string) error {
		id, err := FileID(temp)
		if err != nil {
			return err
		}
		if id != own {
			return errors.New("the copy is not this build")
		}
		return nil
	})
}

// Origin is the directory the image of the executable at path was installed
// in: its own directory, or for a room's copy the one its origin names.
func Origin(path string) string {
	dir := filepath.Dir(path)
	data, err := os.ReadFile(filepath.Join(dir, OriginName))
	if err != nil || filepath.Base(dir) != pinDir {
		return dir
	}
	if origin := strings.TrimSpace(string(data)); filepath.IsAbs(origin) {
		return origin
	}
	return dir
}

// Handoff is the room's own copy when the room runs another build than this
// one and the copy is that build: a call refused here, or a view that would
// read another build's state, is the copy's to answer
// (docs/rules/state.md#the-rooms-own-copy). It writes nothing.
func Handoff(room string) (path, theirs string, ok bool) {
	theirs, err := state.ReadBuildRecord(room)
	if err != nil {
		return "", "", false
	}
	own, err := ID()
	if err != nil || own == theirs {
		return "", "", false
	}
	path = Pinned(room)
	if id, err := FileID(path); err != nil || id != theirs {
		return "", "", false
	}
	return path, theirs, true
}

// HandoffLine is what a call handed to the room's copy says on stderr.
func HandoffLine(room, theirs string) string {
	own, _ := ID()
	return fmt.Sprintf("Rewake: room %s runs build %s since its launch and this is build %s; the room's own copy answers. Restarting main moves the room to this build.", filepath.Base(room), Short(theirs), Short(own))
}
