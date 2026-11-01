// A tool call's head as the face draws it (docs/face.md, "The look"): the
// name Claude Code's terminal gives the tool and its main argument,
// Name(argument), with the name bold; call.ts says which.

import { callOf } from "./call.ts";
import type { Span } from "./messages.ts";
import type { Item } from "./mirror.gen.ts";
import { wrapSpans } from "./wrap.ts";

// A prompt the session holds is drawn under the conversation until it takes
// it (render.ts, statusOf), not where it was sent.
export const hidden = (item: Item) =>
  (item.kind === "tool" && callOf(item).hidden) || item.status === "queued";

// A head longer than this many rows ends in an ellipsis.
const headRows = 3;

// headRowsOf is the call's head after its dot, at most a few rows unless
// the call is shown whole.
export function headRowsOf(item: Item, width: number, whole = false): Span[][] {
  const { name, argument } = callOf(item);
  const spans: Span[] = [{ text: name, bold: true }];
  if (argument) spans.push({ text: `(${argument})` });
  const rows = wrapSpans(spans, width);
  if (whole || rows.length <= headRows) return rows;
  const kept = rows.slice(0, headRows);
  // The last row kept gives a cell to the ellipsis.
  const last = wrapSpans(kept[headRows - 1] ?? [], Math.max(width - 1, 1))[0] ?? [];
  kept[headRows - 1] = [...last, { text: "…" }];
  return kept;
}
