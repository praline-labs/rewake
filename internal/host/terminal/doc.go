// Package terminal is a program's pseudo-terminal, opened with the standard
// library, the few ioctls an attach needs on the person's own terminal — its
// size, raw mode and whether it is one — and the screen the program draws,
// kept by an emulator from a library behind one small interface (Grid). The
// space host relays the program's bytes to an attached terminal as they come
// and feeds the same bytes to a Grid, whose screen main's panel shows
// (docs/v2/workers-panel.md). It is the one package the emulator library
// enters (the owner's exception to the dependency rule,
// docs/v2/stage4-space.md#the-terminal-library), and it knows no harness and
// no mod.
package terminal
