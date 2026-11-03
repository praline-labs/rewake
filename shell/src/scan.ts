// What the widget swallows of a program's output and the shell passes on
// (docs/shell.md, "What the widget lacks"): a copy (OSC 52), the title
// (OSC 0 and 2), and whether the program tracks the mouse, which decides
// whether a drag selects text or goes to the program. Besides, for main's end
// (docs/shell.md, "Who owns which pty"): the program leaving an alternate
// screen it drew on and drawing on one again, and the closing mark main's
// wrapper writes, a private OSC the widget ignores. The scanner is a parser
// of the terminal's own states, fed a byte at a time, so a read may end
// anywhere and the result is the same however the input is cut.

export type Seen = {
  copy?: string;
  title?: string;
  mouse?: boolean;
  left?: boolean;
  entered?: boolean;
  closing?: boolean;
};

// A copy of a selection is up to some megabytes; this bound is far above any
// real one and is only there against an OSC that never ends, so a payload
// longer than it is dropped while the parser goes on through it in its state
// alone.
const payloadLimit = 1 << 26;
// The longest mode number we read has four digits; a parameter of more can
// match none, so no more than this is kept of one.
const modeDigits = 5;
const tracking = new Set(["9", "1000", "1002", "1003"]);
const alternate = new Set(["47", "1047", "1049"]);
const closingMark = "6262;rewake-closing";

// ground draws; esc follows an ESC; escInter an ESC with intermediates; csi
// is a control sequence before its final byte; string is the payload of an
// OSC (kind "]"), DCS ("P"), APC ("_"), PM ("^") or SOS ("X"), and stringEsc
// an ESC met inside one.
type State = "ground" | "esc" | "escInter" | "csi" | "string" | "stringEsc";

// A switch of screens counts once the program draws: main's host replays a
// main on its normal screen with a visit to a hidden alternate screen that
// draws nothing, only to set its margins and saved cursor
// (internal/host/terminal/replay.go). Read as a leave, it covered main with
// the closing layer at every attach of a main not yet on its alternate
// screen, a main that never went there hidden for 5 s (found by the release
// gate, November 3, 2026).
export class Scanner {
  private state: State = "ground";
  private kind = "";
  // payload is an OSC's text so far, or undefined once it passed the limit
  // or for a string that is no OSC.
  private payload: string | undefined = "";
  // A CSI is read as it comes, whatever its length: whether it is a private
  // mode list (`?` first, then digits and `;`), the parameter in hand, and
  // the modes of the list that we read.
  private csiFirst = true;
  private csiPrivate = false;
  private csiParam = "";
  private csiModes = new Set<string>();
  private mouse = new Set<string>();
  // alternate is whether the program is on an alternate screen, and drew
  // whether it has drawn on it since it entered.
  private alternate = false;
  private drew = false;

  scan(data: Uint8Array): Seen[] {
    const seen: Seen[] = [];
    for (const byte of data) this.feed(byte, seen);
    return seen;
  }

  // reset forgets what the program set, for a stream that starts again.
  reset() {
    this.state = "ground";
    this.payload = "";
    this.mouse.clear();
    this.alternate = false;
    this.drew = false;
  }

  private feed(byte: number, seen: Seen[]): void {
    // CAN and SUB cancel any sequence, and what follows is drawn.
    if (byte === 0x18 || byte === 0x1a) {
      this.state = "ground";
      return;
    }
    switch (this.state) {
      case "ground":
        if (byte === 0x1b) this.state = "esc";
        else if (byte >= 0x20 && byte !== 0x7f) this.draw(seen);
        return;
      case "stringEsc":
        // ESC \ is the string's end; any other ESC cancels it and starts
        // a sequence of its own.
        if (byte === 0x5c) {
          this.endString(seen);
          return;
        }
        this.state = "esc";
        this.feed(byte, seen);
        return;
      case "string":
        // BEL ends an OSC alone; in the others it is a control inside.
        if (byte === 0x1b) this.state = "stringEsc";
        else if (byte === 0x07 && this.kind === "]") this.endString(seen);
        else if (this.payload !== undefined) this.take(byte);
        return;
      case "csi":
        if (byte === 0x1b) this.state = "esc";
        else if (byte >= 0x40 && byte <= 0x7e) this.endCsi(String.fromCharCode(byte), seen);
        else if (byte >= 0x20 && byte <= 0x3f) this.param(byte);
        else if (byte >= 0x80) {
          this.state = "ground";
          this.draw(seen);
        }
        return;
      case "escInter":
      case "esc":
        this.escape(byte, seen);
    }
  }

  // escape reads the byte after an ESC, and after its intermediates.
  private escape(byte: number, seen: Seen[]) {
    const c = String.fromCharCode(byte);
    if (byte === 0x1b) this.state = "esc";
    else if (this.state === "esc" && c === "[") {
      this.state = "csi";
      this.csiFirst = true;
      this.csiPrivate = false;
      this.csiParam = "";
      this.csiModes.clear();
    } else if (this.state === "esc" && "]P_^X".includes(c)) {
      this.state = "string";
      this.kind = c;
      this.payload = c === "]" ? "" : undefined;
    } else if (byte >= 0x20 && byte <= 0x2f) this.state = "escInter";
    else if (byte >= 0x30 && byte <= 0x7e) this.state = "ground";
    else if (byte >= 0x80) {
      this.state = "ground";
      this.draw(seen);
    }
  }

  private take(byte: number) {
    const text = this.payload;
    if (text === undefined) return;
    this.payload = text.length < payloadLimit ? text + String.fromCharCode(byte) : undefined;
  }

  // param reads a parameter or intermediate byte of a CSI.
  private param(byte: number) {
    const c = String.fromCharCode(byte);
    if (this.csiFirst) {
      this.csiFirst = false;
      this.csiPrivate = c === "?";
    } else if (!this.csiPrivate) {
      return;
    } else if (c >= "0" && c <= "9") {
      if (this.csiParam.length < modeDigits) this.csiParam += c;
    } else if (c === ";") {
      this.endParam();
    } else {
      this.csiPrivate = false;
    }
  }

  private endParam() {
    if (tracking.has(this.csiParam) || alternate.has(this.csiParam)) {
      this.csiModes.add(this.csiParam);
    }
    this.csiParam = "";
  }

  // draw notes a drawn byte: the first a program draws on an alternate
  // screen is its coming back to it.
  private draw(seen: Seen[]) {
    if (!this.alternate || this.drew) return;
    this.drew = true;
    seen.push({ entered: true });
  }

  private endString(seen: Seen[]) {
    this.state = "ground";
    const text = this.kind === "]" ? this.payload : undefined;
    if (text === undefined) return;
    const copy = /^52;[^;]*;([\s\S]*)$/.exec(text);
    const title = /^[02];([\s\S]*)$/.exec(text);
    if (copy) {
      if (copy[1] !== undefined && copy[1] !== "?")
        seen.push({ copy: Buffer.from(copy[1], "base64").toString("utf8") });
    } else if (title?.[1] !== undefined) {
      seen.push({ title: Buffer.from(title[1], "latin1").toString("utf8") });
    } else if (text === closingMark) {
      seen.push({ closing: true });
    }
  }

  private endCsi(final: string, seen: Seen[]) {
    this.state = "ground";
    if (!this.csiPrivate || (final !== "h" && final !== "l")) return;
    this.endParam();
    const modes = [...this.csiModes];
    const was = this.mouse.size > 0;
    for (const mode of modes) {
      if (!tracking.has(mode)) continue;
      if (final === "h") this.mouse.add(mode);
      else this.mouse.delete(mode);
    }
    if (was !== this.mouse.size > 0) seen.push({ mouse: this.mouse.size > 0 });
    if (!modes.some((mode) => alternate.has(mode))) return;
    // Setting the screen the program is on changes nothing, its drawing
    // included.
    const on = final === "h";
    if (on === this.alternate) return;
    if (!on && this.drew) seen.push({ left: true });
    this.alternate = on;
    this.drew = false;
  }
}
