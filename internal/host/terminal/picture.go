package terminal

// Picture is a program's whole screen kept from the frames that change it,
// its modes included: what the host mode draws a tab from and mirrors the
// input modes of (docs/v2/host-mode.md).
type Picture struct {
	Seq    uint64
	Size   Size
	Rows   [][]Run
	Cursor Cursor
	Modes  Modes
	Title  string
	// Events gather from the frames applied until whoever hands the
	// picture out takes them.
	Events []Event
	// Ended says the program's side is closed: the screen changes no more.
	Ended bool
}

// Apply takes a frame. A new size starts the rows over, since every row comes
// with it; the rows are replaced, never written in place, so a picture handed
// out keeps the rows it had.
func (p *Picture) Apply(s Screen) {
	if s.Size.Cols > 0 && s.Size.Rows > 0 && (s.Size != p.Size || p.Rows == nil) {
		p.Size = s.Size
		p.Rows = make([][]Run, s.Size.Rows)
	} else {
		p.Rows = append([][]Run(nil), p.Rows...)
	}
	for _, row := range s.Changed {
		if row.Index >= 0 && row.Index < len(p.Rows) {
			p.Rows[row.Index] = row.Runs
		}
	}
	p.Seq, p.Cursor, p.Modes, p.Title, p.Ended = s.Seq, s.Cursor, s.Modes, s.Title, p.Ended || s.Ended
	p.Events = append(p.Events, s.Events...)
}
