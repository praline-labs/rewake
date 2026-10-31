// The shell's hold on what a face posts (docs/face.md, "Hot reload"): the
// face is code of its own (face/), REWAKE_FACE may run one from another
// checkout, and what it posts reaches the shell's thread and view.tsx. A
// message is let through only as the contract shapes it, within bounds, so
// no face can make the shell's thread throw; anything else stops the face as
// one that threw.

// The bounds are far past what a face draws — a frame is the pane's rows —
// and keep a broken one from handing the shell's thread unbounded work.
const maxRows = 2000;
const maxSpans = 4096;
const maxText = 16384;
const maxName = 4096;
// A copy is at most the whole conversation, whose model is bounded at 4 MiB.
const maxCopy = 4 * 1024 * 1024;
const maxCount = 1_000_000;

const tones = new Set(["text", "faint", "code", "accent", "done", "error", "gap"]);
const marks = new Set(["added", "removed"]);
const links = new Set(["connecting", "live", "lost"]);
const modes = new Set(["default", "plan", "edits", "auto", "bypass", "unasked"]);

type Fields = Record<string, unknown>;

const isObject = (v: unknown): v is Fields =>
  typeof v === "object" && v !== null && !Array.isArray(v);
const text = (v: unknown, max: number) => typeof v === "string" && v.length <= max;
const maybe = (v: unknown, ok: (v: unknown) => boolean) => v === undefined || ok(v);
const flag = (v: unknown) => maybe(v, (x) => typeof x === "boolean");
const count = (v: unknown) =>
  maybe(v, (x) => Number.isInteger(x) && (x as number) >= 0 && (x as number) <= maxCount);

function spanFault(s: unknown): string | undefined {
  if (!isObject(s)) return "a span that is no object";
  if (!text(s.text, maxText)) return "a span's text";
  if (!maybe(s.tone, (v) => tones.has(v as string))) return "a span's tone";
  if (!maybe(s.mark, (v) => marks.has(v as string))) return "a span's mark";
  if (!maybe(s.glow, (v) => typeof v === "number" && v >= 0 && v <= 1)) return "a span's glow";
  for (const key of ["bold", "italic", "underline", "blink", "gutter", "selected"] as const)
    if (!flag(s[key])) return `a span's ${key}`;
  if (!maybe(s.join, (v) => text(v, maxName))) return "a span's join";
  if (!count(s.cell) || !count(s.code)) return "a span's cell or code";
  return undefined;
}

function lineFault(l: unknown): string | undefined {
  if (!isObject(l)) return "a row that is no object";
  if (!Array.isArray(l.spans) || l.spans.length > maxSpans) return "a row's spans";
  for (const s of l.spans) {
    const fault = spanFault(s);
    if (fault) return fault;
  }
  if (!flag(l.band) || !flag(l.lit)) return "a row's band or light";
  if (!maybe(l.part, (v) => text(v, maxName))) return "a row's part";
  return undefined;
}

// faultOf names what in a message from a face the contract does not allow,
// or nothing when it is one the shell may take.
export function faultOf(m: unknown): string | undefined {
  if (!isObject(m)) return "a message that is no object";
  switch (m.kind) {
    case "lines": {
      if (!Array.isArray(m.lines) || m.lines.length > maxRows) return "its rows";
      for (const l of m.lines) {
        const fault = lineFault(l);
        if (fault) return fault;
      }
      if (!text(m.turn, maxName)) return "its turn";
      if (!links.has(m.link as string)) return "its link";
      if (typeof m.prompts !== "boolean") return "its prompts";
      if (!maybe(m.mode, (v) => modes.has(v as string))) return "its mode";
      return undefined;
    }
    case "error":
      return text(m.text, maxText) ? undefined : "an error's text";
    case "copy":
      return text(m.text, maxCopy) && text(m.what, maxName) ? undefined : "a copy";
    case "answer":
      return text(m.id, maxName) &&
        typeof m.ok === "boolean" &&
        maybe(m.detail, (v) => text(v, maxText))
        ? undefined
        : "an answer";
    case "pong":
      return Number.isSafeInteger(m.n) && (m.n as number) >= 0 ? undefined : "a pong";
    default:
      return "a message of no kind the shell knows";
  }
}
