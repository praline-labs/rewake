package terminal

// Snapshot is the screen as the emulator holds it at one moment: the rows
// of styled runs and the cursor, for a client that wants a picture of the
// screen and no stream (docs/screen.md).
type Snapshot struct {
	Size   Size
	Rows   [][]Run
	Cursor Cursor
}

// Snapshot takes the screen under the grid's lock, between two committed
// pieces of output.
func (g *Grid) Snapshot() Snapshot {
	g.mu.Lock()
	defer g.mu.Unlock()
	l := g.emu.look()
	return Snapshot{Size: l.size, Rows: l.rows, Cursor: l.cursor}
}
