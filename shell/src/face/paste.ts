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

export class Pastes {
  private n = 0;
  private kept = new Map<number, string>();

  // take is what a paste puts in the input: itself, or a large one's marker.
  take(text: string): string {
    const lines = text.split("\n").length;
    if (lines <= largeLines && text.length <= largeChars) return text;
    const id = ++this.n;
    this.kept.set(id, text);
    return lines > largeLines
      ? `[paste #${id} +${lines} lines]`
      : `[paste #${id} ${text.length} chars]`;
  }

  // expand is the input's text with each marker it still holds as what was
  // pasted; a marker typed or left after its paste was forgotten stays.
  expand(text: string): string {
    if (this.kept.size === 0) return text;
    return text.replace(
      /\[paste #(\d+)( (\+\d+ lines|\d+ chars))?\]/g,
      (marker, id: string) => this.kept.get(Number(id)) ?? marker,
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
