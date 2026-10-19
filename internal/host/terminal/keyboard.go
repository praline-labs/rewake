package terminal

import (
	"io"
	"strconv"

	"github.com/charmbracelet/x/ansi"
)

// The keyboard protocols a program may ask its terminal for, kept so the
// host mode can ask the person's terminal for the same (docs/v2/host-mode.md#input):
// the library knows neither, and a program that hears no answer to its kitty
// query falls back to legacy keys, where Shift+Enter is a plain Enter.

const (
	// kittyAll are the progressive enhancements the protocol defines.
	kittyAll = 31
	// kittyStack bounds the saved flags: the protocol asks a terminal to
	// evict the oldest past a limit of its own.
	kittyStack = 16
)

// keyboard registers the kitty keyboard protocol's push, pop, set and query
// (CSI > u, < u, = u, ? u) and xterm's modifyOtherKeys (CSI > 4 ; n m and
// its reset CSI > 4 n). One stack serves both screens, where the protocol
// keeps one each: a program that pushes before it takes the alternate screen
// keeps its keys, which is the forgiving side of the difference.
func (v *vtEmulator) keyboard() {
	v.emu.RegisterCsiHandler(ansi.Command('>', 0, 'u'), func(p ansi.Params) bool {
		flags, _, _ := p.Param(0, 0)
		if len(v.kitty) == kittyStack {
			v.kitty = v.kitty[1:]
		}
		v.kitty = append(v.kitty, v.modes.KittyKeys)
		v.modes.KittyKeys = uint8(flags & kittyAll)
		return true
	})
	v.emu.RegisterCsiHandler(ansi.Command('<', 0, 'u'), func(p ansi.Params) bool {
		n, _, _ := p.Param(0, 1)
		for range max(n, 1) {
			last := len(v.kitty) - 1
			if last < 0 {
				// A pop that empties the stack resets every flag.
				v.modes.KittyKeys = 0
				break
			}
			v.modes.KittyKeys, v.kitty = v.kitty[last], v.kitty[:last]
		}
		return true
	})
	v.emu.RegisterCsiHandler(ansi.Command('=', 0, 'u'), func(p ansi.Params) bool {
		flags, _, _ := p.Param(0, 0)
		set := uint8(flags & kittyAll)
		switch mode, _, _ := p.Param(1, 1); mode {
		case 1:
			v.modes.KittyKeys = set
		case 2:
			v.modes.KittyKeys |= set
		case 3:
			v.modes.KittyKeys &^= set
		}
		return true
	})
	v.emu.RegisterCsiHandler(ansi.Command('?', 0, 'u'), func(ansi.Params) bool {
		_, _ = io.WriteString(v.emu.InputPipe(), "\x1b[?"+strconv.Itoa(int(v.modes.KittyKeys))+"u")
		return true
	})
	v.emu.RegisterCsiHandler(ansi.Command('>', 0, 'm'), func(p ansi.Params) bool {
		// No parameters reset every key modifier option; an omitted value
		// resets the one named.
		which, _, _ := p.Param(0, 4)
		if which == 4 {
			level, _, _ := p.Param(1, 0)
			v.modes.ModifyOtherKeys = uint8(min(max(level, 0), 2))
		}
		return true
	})
	v.emu.RegisterCsiHandler(ansi.Command('>', 0, 'n'), func(p ansi.Params) bool {
		if which, _, _ := p.Param(0, 4); which == 4 {
			v.modes.ModifyOtherKeys = 0
		}
		return true
	})
}
