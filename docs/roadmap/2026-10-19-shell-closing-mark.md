# The closing screen from main's mod

October 19, 2026. **Done.** The owner saw main's pane empty after `/exit` and the spinner half a
second later. Main's mod now tells its wrapper `session/closing` (a new endpoint of the mod↔host
protocol) at `/exit` before the command runs and at `session.end` for Ctrl+C or Ctrl+D; the wrapper
of a main in the shell writes a private OSC on main's pane, and the shell starts the layer on it,
holding main's bytes up to 250 ms after a leave of the alternate screen for the later mark. Live on
2.1.302 with haiku: the spinner at +44–56 ms after `/exit` and +80–132 ms after Ctrl+C, never an
empty pane, against +612–643 ms before. **Open.** Acceptance of the protocol change; the host stops
no worker on it yet (stage 4's plan); Ctrl+Z and an external editor's first frame are 250 ms late.
