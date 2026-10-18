// Package terminal is a program's pseudo-terminal, opened with the standard
// library, and the few ioctls an attach needs on the person's own terminal:
// its size, raw mode and whether it is one. Nothing here reads or draws the
// program's screen: the space host of rewake space open relays the bytes to
// an attached terminal as they come, and that terminal draws them
// (docs/rules/space.md). It knows no harness and no mod.
package terminal
