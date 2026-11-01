// The host's channel (docs/shell.md, "The host's channel"): main's host, the
// process of the person's pane, runs the shell as its child with one end of
// a socket pair on the descriptor REWAKE_HOST_FD names. JSON lines; the
// shell says hello with the version it speaks, and a host of another version
// answers a refusal in a shape no version changes. The Go side is
// internal/host/mainhost/wire.go.
import { writeSync } from "node:fs";

export const version = 1;

// MainRef is main's wrapper and the host socket that serves its stream.
export type MainRef = { pid: number; socket: string };

export type FromHost =
  | { t: "welcome"; generation: number; why: "first" | "requested" | "crashed"; main?: MainRef }
  | { t: "main"; main: MainRef }
  | { t: "go"; state?: unknown }
  | { t: "release" }
  | { t: "closing" }
  | { t: "main-ended"; code: number }
  | { t: "error"; why: string };

export type ToHost =
  | { t: "ready"; colors: string; cols: number; rows: number }
  | { t: "restart" }
  | { t: "close" }
  | { t: "lost" }
  | { t: "released"; state: unknown };

// readLine is one line from the host: a message, or why the channel cannot
// go on — a refusal of this shell's version, or a line that does not read.
export function readLine(line: string): { message: FromHost } | { stop: string } {
  let value: unknown;
  try {
    value = JSON.parse(line);
  } catch {
    return { stop: `the host sent a line that does not read: ${line.slice(0, 200)}` };
  }
  if (typeof value !== "object" || value === null)
    return { stop: `the host sent ${line.slice(0, 200)}` };
  const fields = value as Record<string, unknown>;
  if (typeof fields.refused === "string")
    return { stop: `the host refused this shell: ${fields.refused} (it speaks ${fields.speaks})` };
  if (typeof fields.t !== "string") return { stop: `the host sent ${line.slice(0, 200)}` };
  return { message: value as FromHost };
}

export type Link = { send(message: ToHost): void };

// openLink says hello on fd and hands each message to onMessage; onEnd hears
// once why the channel ended. A write after the end is dropped.
export function openLink(
  fd: number,
  onMessage: (message: FromHost) => void,
  onEnd: (why: string) => void,
): Link {
  let open = true;
  const end = (why: string) => {
    if (!open) return;
    open = false;
    onEnd(why);
  };
  const write = (value: unknown) => {
    if (!open) return;
    try {
      writeSync(fd, `${JSON.stringify(value)}\n`);
    } catch (e) {
      end(`the host's channel failed: ${e}`);
    }
  };
  write({ hello: "shell", v: version });
  void (async () => {
    const reader = Bun.file(fd).stream().getReader();
    const decoder = new TextDecoder();
    let held = "";
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      held += decoder.decode(value, { stream: true });
      for (let nl = held.indexOf("\n"); nl >= 0; nl = held.indexOf("\n")) {
        const line = held.slice(0, nl);
        held = held.slice(nl + 1);
        const read = readLine(line);
        if ("stop" in read) return end(read.stop);
        if (open) onMessage(read.message);
      }
    }
    end("the host's channel ended");
  })().catch((e) => end(`the host's channel failed: ${e}`));
  return { send: write };
}
