# A column of padding in the shell's panes

Each pane's widget sits in a box with one column of padding left and right (no extra rows), so a
program's text does not touch the pane's edge or the divider. The widget's size is the inside, so
the pty size follows it exactly (wide 63/62 of 130 columns, narrow and zoomed width − 2); the
padding is outside the widget, so clicks map to the right cell and a selection never holds it.
A press on the padding chooses the pane. Checked under a pty driver with a fake room: the
capture shows the padding, a resize reaches both programs, a click on a worker's first text
column arrives as column 1, and one in the padding focuses it.
Open: the padding is painted in the terminal's background, so a program's own coloured
background stops one column short of the edge.
