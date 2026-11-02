package terminal

import (
	"fmt"
	"reflect"
	"unsafe"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
)

// The checkpoint is the state of the library's emulator that changes what
// later output does, beyond the cells shown: the screen under the
// alternate one, each screen's margins and saved cursor, the cursor's own
// pen and style, and a pending wrap. A late client's preamble and the next
// image of a host rebuild it from replay (docs/shell.md#the-space-hosts-emulator);
// without it a newline scrolled the wrong region, a restored cursor wrote
// in the wrong place, and a glyph after a pending wrap overwrote the last
// column (found in review, November 2, 2026).
//
// The library keeps all of it unexported, and following it from the bytes
// would rewrite the library's own rules (DECSTBM's defaults, a resize's
// reset of the margins, 1049's save). So it is read from the library: the
// plain fields by reflect, each screen through its own exported methods,
// reached by unsafe. The library is pinned at v0.1.0, and newVT refuses one
// whose fields are gone or retyped (libraryFields), so an update fails every
// test of this package rather than drawing a wrong screen.

// screenState is one of the library's two screens.
type screenState struct {
	screen *vt.Screen
	cursor vt.Cursor
	saved  vt.Cursor
	scroll uv.Rectangle
}

// checkpoint is the emulator's state at one moment.
type checkpoint struct {
	alt     bool
	phantom bool
	screens [2]screenState
}

// libraryFields are the unexported fields the checkpoint reads, by the type
// each must have.
var libraryFields = []struct {
	of    reflect.Type
	field string
	want  reflect.Type
}{
	{reflect.TypeFor[vt.Emulator](), "scrs", reflect.TypeFor[[2]vt.Screen]()},
	{reflect.TypeFor[vt.Emulator](), "atPhantom", reflect.TypeFor[bool]()},
	{reflect.TypeFor[vt.Screen](), "saved", reflect.TypeFor[vt.Cursor]()},
}

// checkLibrary says whether the library still has the fields the
// checkpoint reads.
func checkLibrary() error {
	for _, f := range libraryFields {
		got, ok := f.of.FieldByName(f.field)
		if !ok || got.Type != f.want {
			return fmt.Errorf("the terminal library's %s.%s is not a %s: the screen could not be handed on whole", f.of.Name(), f.field, f.want)
		}
	}
	return nil
}

// checkpoint reads the emulator's state; it runs under the terminal's lock,
// as every read of the emulator does.
func (v *vtEmulator) checkpoint() checkpoint {
	emu := reflect.ValueOf(v.emu).Elem()
	c := checkpoint{alt: v.emu.IsAltScreen(), phantom: emu.FieldByName("atPhantom").Bool()}
	screens := emu.FieldByName("scrs")
	for i := range c.screens {
		field := screens.Index(i)
		screen := (*vt.Screen)(unsafe.Pointer(field.UnsafeAddr()))
		saved := *(*vt.Cursor)(unsafe.Pointer(field.FieldByName("saved").UnsafeAddr()))
		c.screens[i] = screenState{screen: screen, cursor: screen.Cursor(), saved: saved, scroll: screen.ScrollRegion()}
	}
	return c
}

// active is the screen shown.
func (c checkpoint) active() screenState {
	if c.alt {
		return c.screens[1]
	}
	return c.screens[0]
}
