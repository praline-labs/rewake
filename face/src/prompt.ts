// A prompt as the face draws it (docs/face.md, "The look"): the text typed
// on the band behind a faint `❯`, and each letter rewake delivered as its
// notification's summary.
import { hung } from "./hang.ts";
import type { FaceLine, Tone } from "./messages.ts";
import type { Item } from "./mirror.gen.ts";
import { clean } from "./wrap.ts";

// letterRows draws a letter's summary as rewake's notice carries it: its first
// line after the dot, each `↳` preview on its own row under it, the rows hung
// under their text.
function letterRows(said: string, width: number, tone?: Tone): FaceLine[] {
  const [head = "", ...previews] = said
    .split("\n")
    .map((line) => line.replace(/\s+/g, " ").trim())
    .filter((line) => line !== "");
  return [
    ...hung(head, width, { text: "● ", tone }, { text: "" }),
    ...previews.flatMap((line) =>
      line.startsWith("↳")
        ? hung(line.slice(1).trim(), width, { text: "  ↳ " }, { text: "" })
        : hung(line, width, { text: "  " }, { text: "" }),
    ),
  ];
}

// A letter rewake delivered reaches the harness as its notification: the face
// shows its summary, as a terminal program shows the notice it was given, its
// dot by how the task it tells of stands.
const notification = /<task-notification>[\s\S]*?<\/task-notification>/g;
const summary = /<summary>([\s\S]*?)<\/summary>/;
const state = /<status>\s*(\w+)\s*<\/status>/;
const letterTone: Record<string, Tone> = { completed: "done", failed: "error", killed: "gap" };

// A prompt this long shows its head and tail and how many lines between.
const longPrompt = 10_000;
const promptEnd = 2_500;

function shortened(text: string): string {
  if (text.length <= longPrompt) return text;
  const head = text.slice(0, promptEnd);
  const tail = text.slice(-promptEnd);
  const cut = text.slice(promptEnd, -promptEnd).split("\n").length;
  return `${head}\n… +${cut} lines …\n${tail}`;
}

export function promptLines(item: Item, width: number): FaceLine[] {
  const out: FaceLine[] = [];
  const text = clean(item.text);
  for (const m of text.matchAll(notification)) {
    const said = summary.exec(m[0])?.[1] ?? m[0];
    out.push(...letterRows(said, width, letterTone[state.exec(m[0])?.[1] ?? ""]));
  }
  const rest = text.replace(notification, "").trim();
  if (rest || out.length === 0) {
    const tone: Tone | undefined = item.status === "refused" ? "error" : undefined;
    out.push(
      ...hung(
        shortened(rest),
        width,
        { text: "❯ ", tone: tone ?? "faint" },
        { text: "", tone },
        true,
      ),
    );
  }
  return out;
}
