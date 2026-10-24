// Package terminal is a program's pseudo-terminal, opened with the standard
// library, the few ioctls needed on the person's own terminal — whether it is
// one — and the screen the program draws, kept by an emulator from a library
// behind one small interface (Grid), from which the shell's panes are fed
// (docs/shell.md). It is the one package the emulator library enters (the
// owner's exception to the dependency rule,
// docs/shell.md#the-space-hosts-emulator), and it knows no harness and
// no mod.
package terminal
