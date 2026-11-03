// What the widget swallows of a program's output and the shell passes on
// (docs/shell.md, "What the widget lacks"): a copy (OSC 52), the title
// (OSC 0 and 2), and whether the program tracks the mouse, which decides
// whether a drag selects text or goes to the program. Besides, for main's end
// (docs/shell.md, "Who owns which pty"): the program leaving an alternate
// screen it drew on and drawing on one again, and the closing mark main's
// wrapper writes, a private OSC the widget ignores. A read may end inside a
// sequence, so the unfinished tail of one read is carried into the next.

export type Seen = {
  copy?: string;
  title?: string;
  mouse?: boolean;
  left?: boolean;
  entered?: boolean;
  closing?: boolean;
};

// A copy of a long selection is tens of kilobytes; a string longer than this
// is dropped rather than carried without end.
const carryLimit = 1 << 20;
const tracking = new Set(["9", "1000", "1002", "1003"]);
const alternate = new Set(["47", "1047", "1049"]);
// The fifth alternative is the closing mark, as internal/host/start.go
// writes it; the rest are any other CSI, string or escape, none of which
// draws: what lies between the matches is what the program draws.
const sequences =
  /\x1b\[\?([\d;]+)([hl])|\x1b\]52;[^;\x07\x1b]*;([^\x07\x1b]*)(?:\x07|\x1b\\)|\x1b\][02];([^\x07\x1b]*)(?:\x07|\x1b\\)|(\x1b\]6262;rewake-closing\x07)|\x1b\[[0-?]*[ -/]*[@-~]|\x1b[\]P_^X][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[ -/]*[0-OQ-WYZ\\`-~]/g;
const unfinishedEscape = /\x1b(?:\[[0-?]*[ -/]*|[ -/]*)$/;
const unfinishedString = /^\x1b[\]P_^X][^\x07\x1b]*\x1b?$/;
const drawn = /[^\x00-\x1f\x7f]/;

// A switch of screens counts once the program draws: main's host replays a
// main on its normal screen with a visit to a hidden alternate screen that
// draws nothing, only to set its margins and saved cursor
// (internal/host/terminal/replay.go). Read as a leave, it covered main with
// the closing layer at every attach of a main not yet on its alternate
// screen, a main that never went there hidden for 5 s (found by the release
// gate, November 3, 2026).
export class Scanner {
  private carry = "";
  private mouse = new Set<string>();
  // alternate is whether the program is on an alternate screen, and drew
  // whether it has drawn on it since it entered.
  private alternate = false;
  private drew = false;

  scan(data: Uint8Array): Seen[] {
    const text = this.carry + Buffer.from(data).toString("latin1");
    this.carry = tail(text);
    const whole = text.slice(0, text.length - this.carry.length);
    const seen: Seen[] = [];
    let at = 0;
    for (const m of whole.matchAll(sequences)) {
      this.draws(whole.slice(at, m.index), seen);
      at = m.index + m[0].length;
      if (m[1] !== undefined) {
        const was = this.mouse.size > 0;
        for (const mode of m[1].split(";")) {
          if (!tracking.has(mode)) continue;
          if (m[2] === "h") this.mouse.add(mode);
          else this.mouse.delete(mode);
        }
        if (was !== this.mouse.size > 0) seen.push({ mouse: this.mouse.size > 0 });
        if (m[1].split(";").some((mode) => alternate.has(mode))) {
          if (m[2] === "l" && this.alternate && this.drew) seen.push({ left: true });
          this.alternate = m[2] === "h";
          this.drew = false;
        }
      } else if (m[3] !== undefined) {
        if (m[3] !== "?") seen.push({ copy: Buffer.from(m[3], "base64").toString("utf8") });
      } else if (m[4] !== undefined) {
        seen.push({ title: Buffer.from(m[4], "latin1").toString("utf8") });
      } else if (m[5] !== undefined) {
        seen.push({ closing: true });
      }
    }
    this.draws(whole.slice(at), seen);
    return seen;
  }

  // draws notes the text between two sequences: the first a program draws
  // on an alternate screen is its coming back to it.
  private draws(text: string, seen: Seen[]) {
    if (!this.alternate || this.drew || !drawn.test(text)) return;
    this.drew = true;
    seen.push({ entered: true });
  }

  // reset forgets what the program set, for a stream that starts again.
  reset() {
    this.carry = "";
    this.mouse.clear();
    this.alternate = false;
    this.drew = false;
  }
}

// tail is the end of text that begins a sequence it does not finish: a
// string (OSC, DCS, APC, PM, SOS) with no terminator, or a CSI with no final
// byte.
function tail(text: string): string {
  const string = Math.max(
    ...["\x1b]", "\x1bP", "\x1b_", "\x1b^", "\x1bX"].map((s) => text.lastIndexOf(s)),
  );
  if (string >= 0) {
    const rest = text.slice(string);
    if (unfinishedString.test(rest)) return rest.length <= carryLimit ? rest : "";
  }
  return unfinishedEscape.exec(text)?.[0] ?? "";
}
