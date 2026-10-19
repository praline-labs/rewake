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
// reparented has nobody to show anything to.
export function watchTerminal(gone: (why: string) => void) {
  for (const signal of ["SIGHUP", "SIGTERM", "SIGINT", "SIGQUIT", "SIGPIPE"] as const)
    process.on(signal, () => gone(signal));
  process.stdin.on("end", () => gone("the terminal's input ended"));
  process.stdin.on("error", () => gone("the terminal's input failed"));
  const parent = process.ppid;
  setInterval(() => {
    if (process.ppid !== parent) gone("the shell's parent is gone");
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
