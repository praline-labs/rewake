// The shell's hold on what a face posts (docs/face.md, "Hot reload"): the
// face is code of its own (face/), REWAKE_FACE may run one from another
// checkout, and what it posts reaches the shell's thread and view.tsx. A
// message is let through only as the contract shapes it, within bounds, and
// only as a copy the shell builds from the fields it checked: what a face
// sent itself never reaches the shell's tree, since a structured clone keeps
// an array's own extra properties (a `slice` of its own threw in Solid's
// Index; found in review). Anything else stops the face as one that threw.
import type { FaceLine, FromFace, Link, Mark, Span, Tone } from "../../../face/src/messages.ts";
import type { PermissionMode } from "../../../face/src/mirror.gen.ts";

// The bounds are far past what a face draws — a frame is the pane's rows —
// and keep a broken one from handing the shell's thread unbounded work.
const maxRows = 2000;
const maxSpans = 4096;
const maxText = 16384;
const maxName = 4096;
// A copy is at most the whole conversation, whose model is bounded at 4 MiB.
const maxCopy = 4 * 1024 * 1024;
const maxCount = 1_000_000;

const tones = new Set<unknown>(["text", "faint", "code", "accent", "done", "error", "gap"]);
const marks = new Set<unknown>(["added", "removed"]);
const links = new Set<unknown>(["connecting", "live", "lost"]);
const modes = new Set<unknown>(["default", "plan", "edits", "auto", "bypass", "unasked"]);

type Fields = Record<string, unknown>;

// Fault is what the contract does not allow, thrown from deep in a message
// and caught once in taken.
class Fault extends Error {}

const isObject = (v: unknown): v is Fields =>
  typeof v === "object" && v !== null && !Array.isArray(v);
const need = (ok: boolean, what: string) => {
  if (!ok) throw new Fault(what);
};
const text = (v: unknown, max: number) => typeof v === "string" && v.length <= max;
const count = (v: unknown) =>
  Number.isInteger(v) && (v as number) >= 0 && (v as number) <= maxCount;

// listOf reads an array by its indexes alone, into a new one.
function listOf<T>(v: unknown, max: number, what: string, item: (v: unknown) => T): T[] {
  need(Array.isArray(v) && v.length <= max, what);
  const from = v as unknown[];
  const out: T[] = [];
  for (let i = 0; i < from.length; i++) out.push(item(from[i]));
  return out;
}

const flags = ["bold", "italic", "underline", "blink", "gutter", "selected"] as const;

function spanOf(s: unknown): Span {
  need(isObject(s), "a span that is no object");
  const f = s as Fields;
  need(text(f.text, maxText), "a span's text");
  const span: Span = { text: f.text as string };
  if (f.tone !== undefined) {
    need(tones.has(f.tone), "a span's tone");
    span.tone = f.tone as Tone;
  }
  if (f.mark !== undefined) {
    need(marks.has(f.mark), "a span's mark");
    span.mark = f.mark as Mark;
  }
  if (f.glow !== undefined) {
    need(typeof f.glow === "number" && f.glow >= 0 && f.glow <= 1, "a span's glow");
    span.glow = f.glow as number;
  }
  for (const key of flags) {
    if (f[key] === undefined) continue;
    need(typeof f[key] === "boolean", `a span's ${key}`);
    span[key] = f[key] as boolean;
  }
  if (f.join !== undefined) {
    need(text(f.join, maxName), "a span's join");
    span.join = f.join as string;
  }
  for (const key of ["cell", "code"] as const) {
    if (f[key] === undefined) continue;
    need(count(f[key]), "a span's cell or code");
    span[key] = f[key] as number;
  }
  return span;
}

function lineOf(l: unknown): FaceLine {
  need(isObject(l), "a row that is no object");
  const f = l as Fields;
  const line: FaceLine = { spans: listOf(f.spans, maxSpans, "a row's spans", spanOf) };
  for (const key of ["band", "lit"] as const) {
    if (f[key] === undefined) continue;
    need(typeof f[key] === "boolean", "a row's band or light");
    line[key] = f[key] as boolean;
  }
  if (f.part !== undefined) {
    need(text(f.part, maxName), "a row's part");
    line.part = f.part as string;
  }
  return line;
}

function messageOf(m: unknown): FromFace {
  need(isObject(m), "a message that is no object");
  const f = m as Fields;
  switch (f.kind) {
    case "lines": {
      const lines = listOf(f.lines, maxRows, "its rows", lineOf);
      need(text(f.turn, maxName), "its turn");
      need(links.has(f.link), "its link");
      need(typeof f.prompts === "boolean", "its prompts");
      need(f.mode === undefined || modes.has(f.mode), "its mode");
      const message: FromFace = {
        kind: "lines",
        lines,
        turn: f.turn as string,
        link: f.link as Link,
        prompts: f.prompts as boolean,
      };
      if (f.mode !== undefined) message.mode = f.mode as PermissionMode;
      return message;
    }
    case "error":
      need(text(f.text, maxText), "an error's text");
      return { kind: "error", text: f.text as string };
    case "copy":
      need(text(f.text, maxCopy) && text(f.what, maxName), "a copy");
      return { kind: "copy", text: f.text as string, what: f.what as string };
    case "answer": {
      need(
        text(f.id, maxName) &&
          typeof f.ok === "boolean" &&
          (f.detail === undefined || text(f.detail, maxText)),
        "an answer",
      );
      const answer: FromFace = { kind: "answer", id: f.id as string, ok: f.ok as boolean };
      if (f.detail !== undefined) answer.detail = f.detail as string;
      return answer;
    }
    case "pong":
      need(Number.isSafeInteger(f.n) && (f.n as number) >= 0, "a pong");
      return { kind: "pong", n: f.n as number };
    default:
      throw new Fault("a message of no kind the shell knows");
  }
}

// taken is a message from a face as the shell's own copy, built from the
// fields the contract names, or the fault that stops the face.
export function taken(m: unknown): { message: FromFace } | { fault: string } {
  try {
    return { message: messageOf(m) };
  } catch (err) {
    if (err instanceof Fault) return { fault: err.message };
    throw err;
  }
}

export const faultOf = (m: unknown): string | undefined => {
  const t = taken(m);
  return "fault" in t ? t.fault : undefined;
};
