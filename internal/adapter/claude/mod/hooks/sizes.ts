// The sizes of the mirror's texts as the wrapper counts them (mirror.ts):
// a text within its bound is sent whole, past it is cut or split between
// characters, never inside one.
// jsonSize is a text's size inside a JSON string, as the wrapper counts it:
// the UTF-8 bytes, with the escapes JSON.stringify writes.
export function jsonSize(text: string): number {
  let size = 0;
  for (let i = 0; i < text.length; i++) {
    size += charSize(text, i);
    if (isPair(text, i)) i++;
  }
  return size;
}

function isPair(text: string, i: number): boolean {
  const c = text.charCodeAt(i);
  if (c < 0xd800 || c > 0xdbff || i + 1 >= text.length) return false;
  const d = text.charCodeAt(i + 1);
  return d >= 0xdc00 && d <= 0xdfff;
}

function charSize(text: string, i: number): number {
  const c = text.charCodeAt(i);
  if (c === 0x22 || c === 0x5c || c === 0x0a || c === 0x0d || c === 0x09) return 2;
  if (c < 0x20) return 6;
  if (c < 0x80) return 1;
  if (c < 0x800) return 2;
  if (isPair(text, i)) return 4;
  if (c >= 0xd800 && c <= 0xdfff) return 6;
  return 3;
}

// utf8Size is a serialized value's size on the wire: JSON.stringify leaves
// no control character or lone surrogate unescaped, so its UTF-8 bytes are
// the body's.
export function utf8Size(serialized: string): number {
  let size = 0;
  for (let i = 0; i < serialized.length; i++) {
    const c = serialized.charCodeAt(i);
    if (c < 0x80) size += 1;
    else if (c < 0x800) size += 2;
    else if (isPair(serialized, i)) {
      size += 4;
      i++;
    } else size += 3;
  }
  return size;
}

// pieces splits a text into pieces each within budget, between characters.
export function pieces(text: string, budget: number): string[] {
  const out: string[] = [];
  let start = 0;
  let size = 0;
  for (let i = 0; i < text.length; i++) {
    const width = isPair(text, i) ? 2 : 1;
    const next = charSize(text, i);
    if (size + next > budget && i > start) {
      out.push(text.slice(start, i));
      start = i;
      size = 0;
    }
    size += next;
    i += width - 1;
  }
  if (start < text.length || out.length === 0) out.push(text.slice(start));
  return out;
}

const cutMark = "\n… cut …\n";

// fit keeps a text within budget: whole, or its head, a mark, and its tail.
export function fit(text: string, budget: number): string {
  if (jsonSize(text) <= budget) return text;
  let head = 0;
  for (let size = 0; head < text.length; ) {
    const next = charSize(text, head);
    if (size + next > budget / 2) break;
    size += next;
    head += isPair(text, head) ? 2 : 1;
  }
  let tail = text.length;
  for (let size = 0; tail > head; ) {
    const at = tail >= 2 && isPair(text, tail - 2) ? tail - 2 : tail - 1;
    const next = charSize(text, at);
    if (size + next > budget / 4) break;
    size += next;
    tail = at;
  }
  return text.slice(0, head) + cutMark + text.slice(tail);
}
