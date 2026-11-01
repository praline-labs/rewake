// The shell's hold on what a face posts (docs/face.md, "Hot reload"): the
// face is code of its own (face/), REWAKE_FACE may run one from another
// checkout, and what it posts reaches the shell's thread and view.tsx. A
// message is let through only as the contract shapes it — the shapes of
// face/src/messages.ts, built into the shell whatever face runs — within
// bounds, and only as a copy the shell builds from the fields it checked:
// what a face sent itself never reaches the shell's tree, since a structured
// clone keeps an array's own extra properties (a `slice` of its own threw in
// Solid's Index; found in review). Anything else stops the face as one that
// threw.
import { type FromFace, fromFace } from "../../../face/src/messages.ts";
import type { Bound, Field, Shape } from "../../../face/src/shape.ts";

// The bounds are far past what a face draws — a frame is the pane's rows —
// and keep a broken one from handing the shell's thread unbounded work.
const bounds: Record<Bound, number> = {
  rows: 2000,
  spans: 4096,
  text: 16384,
  name: 4096,
  // A copy is at most the whole conversation, whose model is bounded at 4 MiB.
  copy: 4 * 1024 * 1024,
};
const maxCount = 1_000_000;

type Fields = Record<string, unknown>;

// Fault is what the contract does not allow, thrown from deep in a message
// and caught once in taken.
class Fault extends Error {}

const isObject = (v: unknown): v is Fields =>
  typeof v === "object" && v !== null && !Array.isArray(v);
const need = (ok: boolean, what: string) => {
  if (!ok) throw new Fault(what);
};

// fieldOf is a field's value as its shape allows it, a list read by its
// indexes alone into a new one; what names the field in a fault.
function fieldOf(field: Field, v: unknown, what: string): unknown {
  switch (field.is) {
    case "text":
      need(typeof v === "string" && v.length <= bounds[field.bound], what);
      break;
    case "flag":
      need(typeof v === "boolean", what);
      break;
    case "count":
      need(Number.isInteger(v) && (v as number) >= 0 && (v as number) <= maxCount, what);
      break;
    case "serial":
      need(Number.isSafeInteger(v) && (v as number) >= 0, what);
      break;
    case "one":
      need(typeof v === "string" && field.of.includes(v), what);
      break;
    case "list": {
      need(Array.isArray(v) && v.length <= bounds[field.bound], what);
      const from = v as unknown[];
      const out: Fields[] = [];
      for (let i = 0; i < from.length; i++) out.push(objectOf(field.of, from[i], field.item));
      return out;
    }
  }
  return v;
}

// objectOf is a new object of the fields a shape names, each as it allows;
// whatever else the face put on it is left behind.
function objectOf(shape: Shape, v: unknown, owner: string): Fields {
  need(isObject(v), `${owner} that is no object`);
  const from = v as Fields;
  const out: Fields = {};
  for (const [key, field] of Object.entries(shape)) {
    if (from[key] === undefined && field.optional) continue;
    out[key] = fieldOf(field, from[key], `${owner}'s ${key}`);
  }
  return out;
}

function messageOf(m: unknown): FromFace {
  need(isObject(m), "a message that is no object");
  const kind = (m as Fields).kind;
  need(
    typeof kind === "string" && Object.hasOwn(fromFace, kind),
    "a message of no kind the shell knows",
  );
  const shapes: Record<string, Shape> = fromFace;
  return { kind, ...objectOf(shapes[kind as string] ?? {}, m, `the ${kind} message`) } as FromFace;
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
