// Package terminal is a program's pseudo-terminal and the screen it draws:
// the pair opened with the standard library, the program's output fed to an
// emulator from a library behind one small interface, and the screen read
// back as rows of styled runs with the cursor and the modes a key encoder
// needs (docs/v2/stage4-space.md#1-the-pieces-and-where-they-live). It is the
// one package the emulator library enters (the owner's exception to the
// dependency rule, docs/v2/stage4-space.md#the-terminal-library), and it
// knows no harness and no mod: the space host maps its screen onto the
// protocol. Plain is the pair alone, with no emulator: the space host of
// rewake space open relays its bytes to an attached terminal as they come.
package terminal
