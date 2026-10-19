// What the widget swallows of a program's output and the shell passes on
// (docs/v2/shell.md, "What the widget lacks"): a copy (OSC 52), the title
// (OSC 0 and 2), and whether the program tracks the mouse, which decides
// whether a drag selects text or goes to the program. Besides, for main's end
// (docs/v2/shell.md, "Who owns which pty"): the program leaving the alternate
// screen, and the closing mark main's wrapper writes, a private OSC the
// widget ignores. A read may end inside a sequence, so the unfinished tail of
// one read is carried into the next.

export type Seen = {
  copy?: string;
  title?: string;
  mouse?: boolean;
  left?: boolean;
  closing?: boolean;
};

// A copy of a long selection is tens of kilobytes; a string longer than this
// is dropped rather than carried without end.
const carryLimit = 1 << 20;
const tracking = new Set(["9", "1000", "1002", "1003"]);
const alternate = new Set(["47", "1047", "1049"]);
// The last alternative is the closing mark, as internal/host/start.go
// writes it.
const sequences =
  /\x1b\[\?([\d;]+)([hl])|\x1b\]52;[^;\x07\x1b]*;([^\x07\x1b]*)(?:\x07|\x1b\\)|\x1b\][02];([^\x07\x1b]*)(?:\x07|\x1b\\)|(\x1b\]6262;rewake-closing\x07)/g;
const unfinishedCsi = /\x1b(\[[\d;?]*)?$/;

export class Scanner {
  private carry = "";
  private mouse = new Set<string>();

  scan(data: Uint8Array): Seen[] {
    const text = this.carry + Buffer.from(data).toString("latin1");
    const seen: Seen[] = [];
    for (const m of text.matchAll(sequences)) {
      if (m[1] !== undefined) {
        const was = this.mouse.size > 0;
        for (const mode of m[1].split(";")) {
          if (!tracking.has(mode)) continue;
          if (m[2] === "h") this.mouse.add(mode);
          else this.mouse.delete(mode);
        }
        if (was !== this.mouse.size > 0) seen.push({ mouse: this.mouse.size > 0 });
        if (m[2] === "l" && m[1].split(";").some((mode) => alternate.has(mode)))
          seen.push({ left: true });
      } else if (m[3] !== undefined) {
        if (m[3] !== "?") seen.push({ copy: Buffer.from(m[3], "base64").toString("utf8") });
      } else if (m[4] !== undefined) {
        seen.push({ title: Buffer.from(m[4], "latin1").toString("utf8") });
      } else if (m[5] !== undefined) {
        seen.push({ closing: true });
      }
    }
    this.carry = tail(text);
    return seen;
  }

  // reset forgets what the program set, for a stream that starts again.
  reset() {
    this.carry = "";
    this.mouse.clear();
  }
}

// tail is the end of text that begins a sequence it does not finish: an OSC
// string with no terminator, or a CSI with no final byte.
function tail(text: string): string {
  const osc = text.lastIndexOf("\x1b]");
  if (osc >= 0) {
    const rest = text.slice(osc);
    if (!rest.includes("\x07") && !rest.includes("\x1b\\"))
      return rest.length <= carryLimit ? rest : "";
  }
  return unfinishedCsi.exec(text)?.[0] ?? "";
}
