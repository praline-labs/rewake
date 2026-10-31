// The input's top rule (docs/face.md, "The look"): the session's permission
// mode at its left end, as the terminal program names it under its input,
// and the session's name with its context at the right, `── name ─` as the
// terminal program draws its own. A narrow pane drops the mode first, then
// the context, then cuts the name with …; whatever its role, a session's
// rule is drawn the same.

import type { PermissionMode } from "../../../face/src/mirror.gen.ts";

// RulePart is a run of the rule's text, in the rule's colour unless it
// names the mode, drawn in that mode's colour.
export type RulePart = { text: string; mode?: PermissionMode };

// modeNames are the modes as the terminal program shows them; the default
// one says nothing.
export const modeNames: Partial<Record<PermissionMode, string>> = {
  auto: "⏵⏵ auto mode",
  plan: "⏸ plan mode",
  edits: "⏵⏵ accept edits",
  bypass: "⏵⏵ bypass permissions",
  unasked: "⏵⏵ don't ask",
};

const graphemes = new Intl.Segmenter();
const line = (n: number) => "─".repeat(Math.max(n, 0));
const width = (text: string) => Bun.stringWidth(text);

// cut is the name within room cells, cut with … where it is longer.
function cut(title: string, room: number): string {
  if (width(title) <= room) return title;
  let name = "";
  for (const { segment } of graphemes.segment(title)) {
    if (width(name + segment) > room - 1) break;
    name += segment;
  }
  return `${name}…`;
}

// ruleParts is the rule cells wide: `── mode ` at its left, ` name · context ─`
// at its right, at least a cell of the rule between them and two before a
// name alone. Each try leaves out one more of the mode and the context; the
// last cuts the name.
export function ruleParts(
  cells: number,
  title?: string,
  context?: string,
  mode?: PermissionMode,
): RulePart[] {
  const label = mode ? modeNames[mode] : undefined;
  const tries: [string | undefined, string | undefined][] = [];
  if (label) tries.push([label, title ? context : undefined]);
  if (title && context) tries.push([undefined, context]);
  for (const [shown, ctx] of tries) {
    const left = shown ? `── ${shown} ` : "";
    const right = title ? ` ${title}${ctx ? ` · ${ctx}` : ""} ─` : "";
    const fill = cells - width(left) - width(right);
    if (fill < (shown ? 1 : 2)) continue;
    return shown
      ? [{ text: "── " }, { text: shown, mode }, { text: ` ${line(fill)}${right}` }]
      : [{ text: line(fill) + right }];
  }
  // Two cells of the rule before the name, a space, the name, a space and a
  // cell of the rule after it.
  const room = cells - 5;
  if (!title || room < 2) return [{ text: line(cells) }];
  const tail = ` ${cut(title, room)} ─`;
  return [{ text: line(cells - width(tail)) + tail }];
}

// ruleOf is the rule's text alone.
export const ruleOf = (cells: number, title?: string, context?: string, mode?: PermissionMode) =>
  ruleParts(cells, title, context, mode)
    .map((p) => p.text)
    .join("");
