// The shell's own end (docs/shell.md, "Restart, crash and close"): main's
// host ends main, and the shell tells it why — a stop signal is a close, a
// terminal gone is a loss, after which there is nothing to draw on.
//
// OpenTUI's own signal handlers only destroy the renderer: the process lived
// on after its terminal was killed. So the renderer is made with no exit
// signals, and the shell follows its terminal itself.

// watchTerminal calls gone once the person's terminal is lost: a hangup or
// another ending signal, the input at an end, or the parent gone — the host
// is the shell's parent, and a shell reparented has nobody to show anything
// to. lost says the terminal itself is gone, so there is nothing to draw the
// closing on; SIGTERM, SIGINT and SIGQUIT end the shell with its terminal
// still there.
export function watchTerminal(gone: (why: string, lost: boolean) => void) {
  for (const signal of ["SIGHUP", "SIGPIPE"] as const) process.on(signal, () => gone(signal, true));
  for (const signal of ["SIGTERM", "SIGINT", "SIGQUIT"] as const)
    process.on(signal, () => gone(signal, false));
  process.stdin.on("end", () => gone("the terminal's input ended", true));
  process.stdin.on("error", () => gone("the terminal's input failed", true));
  const parent = process.ppid;
  setInterval(() => {
    if (process.ppid !== parent) gone("the shell's parent is gone", true);
  }, 1000).unref();
}
