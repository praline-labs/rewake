# The shell keeps the terminal's own foreground

October 19, 2026. **Done.** The owner saw the shell "still black" after the transparent
background. The shell runs inside herdr 0.9.3, which answered OSC 4 but not OSC 10/11, so the
panes kept the terminal's background (SGR 49, read live from herdr's copy of the pane) but the
widget wrote its unseeded default foreground as white. The widget is now seeded with a stand-in
grey and its cells in that colour are given the default-foreground intent after it draws, written
as SGR 39. Checked under a stand-in terminal with no answer, OSC 4 only, and all answers, and with
herdr between: the panes write 39 and 49 unless the terminal named its colours. **Open.** The
owner's look on herdr; a program that asks the widget for OSC 10/11 hears the stand-in grey and black.
