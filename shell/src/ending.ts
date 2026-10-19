// Closing ends everything (docs/v2/shell.md, "Who owns which pty"): the
// shell ends when main ends and when its own terminal goes, and ending it
// hangs up main, whose run's end ends every worker its space hosts keep.
//
// OpenTUI's own signal handlers only destroy the renderer: the process lived
// on with both sessions after its terminal was killed, the pane's ptys
// keeping it alive. So the renderer is made with no exit signals, and the
// shell follows its terminal itself.
import type { Subprocess } from "bun";

const hangupWait = 5000;
const termWait = 3000;

// watchTerminal calls gone once the person's terminal is lost: a hangup or
// another ending signal, the input at an end, or the parent gone — a
// terminal window closed takes the shell's parent with it, and a shell
// reparented has nobody to show anything to. lost says the terminal itself
// is gone, so there is nothing to draw the closing on; SIGTERM, SIGINT and
// SIGQUIT end the shell with its terminal still there.
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

const sleep = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms));

function signalGroup(pid: number, signal: NodeJS.Signals) {
  try {
    process.kill(-pid, signal);
  } catch {
    // The group is gone already.
  }
}

// endMain hangs main up by closing its pty, then waits a bound for it before
// SIGTERM and SIGKILL to its whole group.
export async function endMain(child: Subprocess, hangup: () => void): Promise<number> {
  hangup();
  const exited = (wait: number) =>
    Promise.race([child.exited.then(() => true), sleep(wait).then(() => false)]);
  if (!(await exited(hangupWait))) {
    signalGroup(child.pid, "SIGTERM");
    if (!(await exited(termWait))) signalGroup(child.pid, "SIGKILL");
  }
  return child.exitCode ?? 1;
}

// tailText is the last lines of what main wrote, its escapes taken out: what
// a launch that failed inside the shell said, shown once the shell left the
// screen, or the person would see the shell vanish without a word.
export function tailText(raw: string, lines = 20): string {
  const text = raw
    .replace(/\x1b\][^\x07\x1b]*(\x07|\x1b\\)|\x1b\[[0-?]*[ -/]*[@-~]|\x1b[@-_]/g, "")
    .split(/\r?\n|\r/)
    .map((line) => line.trimEnd())
    .filter((line) => line !== "");
  return text.slice(-lines).join("\n");
}

// LeaveHold holds main's bytes from the moment main leaves the alternate
// screen, for at most bound ms, so that the closing mark which follows
// Ctrl+C or Ctrl+D some 60 ms later (docs/v2/shell.md) starts the closing
// layer before main's pane shows the screen main left to. Ctrl+Z and an
// external editor leave it too: their first frame is late by the bound.
export class LeaveHold {
  private held: Uint8Array[] = [];
  private timer: ReturnType<typeof setTimeout> | undefined;

  constructor(
    private write: (data: Uint8Array) => void,
    private bound = 250,
  ) {}

  // pass writes data at once, or holds it while a leave is held, left
  // saying data leaves the alternate screen.
  pass(data: Uint8Array, left: boolean) {
    if (left && this.timer === undefined) this.timer = setTimeout(() => this.release(), this.bound);
    if (this.timer === undefined) this.write(data);
    else this.held.push(data);
  }

  // release writes what is held and ends the hold.
  release() {
    clearTimeout(this.timer);
    this.timer = undefined;
    for (const data of this.held.splice(0)) this.write(data);
  }
}
