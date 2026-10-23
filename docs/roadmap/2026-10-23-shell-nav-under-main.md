# The shell's navigation under main's pane

October 23, 2026, on the owner's decision ([shell.md](../v2/shell.md#the-bar)). **Done; the owner's look open.**
While both panes show, the bar and the hints sit under main's pane, in main's column; the worker's column takes the
whole height with its strip as the last row, so both columns end together. While one pane shows — narrow or zoomed —
they take the two bottom lines across the whole width as before. The hints centre in main's column and are cut from
the right there; `Nav` in `shell/src/bar.tsx` draws both lines at the width it is given. Live under a stand-in
terminal with stand-in main and space hosts: wide with main or a worker focused, the prefix waiting, 124 columns,
narrow, zoomed; clicks on the bar chose a worker wide and narrow. **Open:** the owner's look on their terminal.
