// Package terminal is a program's pseudo-terminal, opened with the standard
// library, the few ioctls the host mode needs on the person's own terminal —
// its size, raw mode and whether it is one — the screen the program draws,
// kept by an emulator from a library behind one small interface (Grid),
// which the host mode draws from (docs/v2/host-mode.md). It is the one package the emulator library
// enters (the owner's exception to the dependency rule,
// docs/v2/stage4-space.md#the-terminal-library), and it knows no harness and
// no mod.
package terminal
