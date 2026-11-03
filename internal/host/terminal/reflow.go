package terminal

import (
	"strconv"

	uv "github.com/charmbracelet/ultraviolet"
)

// resize sets the size. The library cuts a row at a narrower width and keeps
// nothing of the cut, but a terminal wraps what the row held onto the rows
// below, and the person's widget does: a line main wrote across the window,
// then shown in a pane half as wide, is two lines there, and a replay of the
// cut row would show one (docs/shell.md#the-space-hosts-emulator).
func (v *vtEmulator) resize(size Size, pending []byte) {
	if size.Cols >= v.emu.Width() || v.emu.IsAltScreen() {
		// A program on the alternate screen draws it again at its new size.
		v.emu.Resize(size.Cols, size.Rows)
		return
	}
	rows, cursor := v.wrapped(size)
	v.emu.Resize(size.Cols, size.Rows)
	empty := uv.EmptyCell
	for y := range size.Rows {
		for x := range size.Cols {
			v.emu.SetCell(x, y, &empty)
		}
	}
	for y, row := range rows {
		for x, cell := range row {
			if !blankCell(&cell) {
				v.emu.SetCell(x, y, &cell)
			}
		}
	}
	// Margins are whole after a resize, so the position is the screen's. The
	// library places the cursor only by a sequence, which ends the one the
	// program left unfinished, so that one's bytes are given again.
	_, _ = v.emu.Write([]byte("\x1b[" + strconv.Itoa(cursor.Y+1) + ";" + strconv.Itoa(cursor.X+1) + "H"))
	v.write(pending)
}

// wrapped is the shown screen's rows cut to the width of size and wrapped
// where they are longer, fitted to its height, and where the cursor then is.
// The library keeps no mark of a row the program's autowrap broke, so a row
// that fills the width and has a row of text after it is taken as broken and
// joined to it before the wrap, as the width the program wrote at was the
// first to wrap it. A row is cut at the glyph before the edge, never through
// a wide one.
func (v *vtEmulator) wrapped(size Size) ([][]uv.Cell, uv.Position) {
	was := v.size()
	at := v.emu.CursorPosition()
	var rows [][]uv.Cell
	cursor := uv.Position{}
	for y := 0; y < was.Rows; {
		var line []uv.Cell
		first, column := y, 0
		for {
			cells, used := v.rowCells(y)
			if y == at.Y {
				column = len(line) + at.X
			}
			line = append(line, cells[:used]...)
			y++
			if used < was.Cols || y >= was.Rows {
				break
			}
			if _, next := v.rowCells(y); next == 0 {
				break
			}
		}
		start := 0
		for {
			end := min(start+size.Cols, len(line))
			if end < len(line) && line[end].Width == 0 && end-1 > start {
				end--
			}
			rows = append(rows, line[start:end])
			if column >= start && at.Y >= first && at.Y < y {
				cursor = uv.Position{X: min(column-start, size.Cols-1), Y: len(rows) - 1}
			}
			if start = end; start >= len(line) {
				break
			}
		}
	}
	// Rows below the cursor that hold nothing go first, then the top ones,
	// which a terminal would scroll off; the cursor stays on the screen.
	for len(rows)-1 > cursor.Y && len(rows[len(rows)-1]) == 0 {
		rows = rows[:len(rows)-1]
	}
	if over := min(len(rows)-size.Rows, cursor.Y); over > 0 {
		rows, cursor.Y = rows[over:], cursor.Y-over
	}
	if len(rows) > size.Rows {
		rows = rows[:size.Rows]
	}
	return rows, cursor
}

// rowCells is row y's cells and how many of them end at the last that shows
// something.
func (v *vtEmulator) rowCells(y int) ([]uv.Cell, int) {
	cells := make([]uv.Cell, v.emu.Width())
	used := 0
	for x := range cells {
		if cell := v.emu.CellAt(x, y); cell != nil {
			cells[x] = *cell
			if !blankCell(cell) {
				used = x + 1
			}
		}
	}
	return cells, used
}

// blankCell is a cell that shows nothing.
func blankCell(c *uv.Cell) bool {
	return (c.Content == "" || c.Content == " ") && c.Style.IsZero() && c.Link == uv.Link{}
}
