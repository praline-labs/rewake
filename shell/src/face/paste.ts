// The face's input as pi's editor behaves (docs/face.md, "The input"), as
// read in the bundled pi-tui 1.4.0's editor.js: how many rows it grows to,
// how a paste is cleaned, and a large paste kept whole behind a marker the
// input shows in its place, `[paste #1 +40 lines]`, put back when it is sent.

// rowsFor is how many rows the input grows to before it scrolls inside: 30%
// of the pane's rows, at least 5, as pi's editor takes 30% of its terminal.
export const rowsFor = (height: number) => Math.max(5, Math.floor(height * 0.3));

// cleaned is a paste as pi's editor takes one: line ends made \n, a tab four
// spaces, and no control character but the newline.
export const cleaned = (text: string) =>
  text
    .replace(/\r\n/g, "\n")
    .replace(/\r/g, "\n")
    .replace(/\t/g, "    ")
    // biome-ignore lint/suspicious/noControlCharactersInRegex: a paste's control characters are dropped.
    .replace(/[\u0000-\u0009\u000b-\u001f\u007f]/g, "");

// A paste past either is large: kept whole, a marker shown in its place.
const largeLines = 10;
const largeChars = 1000;

// marker finds pi's paste markers in a text.
const marker = /\[paste #(\d+)( (\+\d+ lines|\d+ chars))?\]/g;

// Paste is what a paste puts in the input: its text, or a large one's
// marker and the paste it stands for.
export type Paste = { text: string; id?: number };

export class Pastes {
  private n = 0;
  private kept = new Map<number, string>();

  // take is what a paste puts in the input: itself, or a large one's marker.
  take(text: string): Paste {
    const lines = text.split("\n").length;
    if (lines <= largeLines && text.length <= largeChars) return { text };
    const id = ++this.n;
    this.kept.set(id, text);
    const shown = lines > largeLines ? `+${lines} lines` : `${text.length} chars`;
    return { text: `[paste #${id} ${shown}]`, id };
  }

  // markers are the markers in a text whose paste is kept, where each stands.
  markers(text: string): { id: number; start: number; end: number }[] {
    if (this.kept.size === 0) return [];
    return [...text.matchAll(marker)]
      .map((m) => ({ id: Number(m[1]), start: m.index, end: m.index + m[0].length }))
      .filter((m) => this.kept.has(m.id));
  }

  // expand is the input's text with each marker of the ids still in it as
  // what was pasted; a marker typed, or one of a paste deleted, stays.
  expand(text: string, live: Set<number>): string {
    return text.replace(marker, (shown, id: string) =>
      live.has(Number(id)) ? (this.kept.get(Number(id)) ?? shown) : shown,
    );
  }

  // clear forgets the pastes once the prompt went, as pi's editor does.
  clear() {
    this.n = 0;
    this.kept.clear();
  }
}

// moreRule is the rule above or under the input while its text scrolls
// inside it: `↑ 3 more` or `↓ 2 more` in its middle, as pi's editor draws
// its borders; the plain rule while nothing is hidden that way.
export function moreRule(cells: number, arrow: "↑" | "↓", hidden: number): string {
  const line = (n: number) => "─".repeat(Math.max(n, 0));
  if (hidden <= 0) return line(cells);
  const label = ` ${arrow} ${hidden} more `;
  const room = cells - Bun.stringWidth(label);
  if (room < 2) return line(cells);
  const left = Math.floor(room / 2);
  return line(left) + label + line(room - left);
}
