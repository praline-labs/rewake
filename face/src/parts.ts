// The parts a click copies whole (docs/face.md, "Copying"): a reply as its
// markdown source, a code block's code, a thinking block's text, a call's
// input — a command, a path, a pattern, else the input as given — a call's
// output, an edit's old or new lines, a written file's lines. A row names its
// part by a key (select.ts); the text is taken from the model when the click
// comes, so the rows carry no copy of it.
import { callOf } from "./call.ts";
import { codeBlocks } from "./markdown.ts";
import type { Item } from "./mirror.gen.ts";

// edits is an edit's old or new strings, one after another.
const edits = (item: Item, key: "old" | "new") =>
  (callOf(item).edits ?? [])
    .map((e) => e[key])
    .filter((s): s is string => s !== undefined)
    .join("\n");

// partOf is the text of the part a key names and what to call it, none when
// its item is gone or the part is empty.
export function partOf(items: Item[], key: string): { text: string; what: string } | undefined {
  const at = key.indexOf(" ");
  const kind = key.slice(0, at);
  const id = key.slice(at + 1);
  const item = items.find((i) => i.id === id);
  if (!item) return undefined;
  const part = ((): { text: string; what: string } | undefined => {
    const code = /^code(\d+)$/.exec(kind);
    if (code) return { text: codeBlocks(item.text)[Number(code[1])] ?? "", what: "the code block" };
    switch (kind) {
      case "reply":
        return { text: item.text, what: "the reply" };
      case "thinking":
        return { text: item.text, what: "the thinking" };
      case "input":
        return callOf(item).copy;
      case "output":
        return { text: item.output ?? "", what: "the call's output" };
      case "old":
        return { text: edits(item, "old"), what: "the old lines" };
      case "new":
        return { text: edits(item, "new"), what: "the new lines" };
      case "content":
        return { text: callOf(item).written?.content ?? "", what: "the file's lines" };
    }
    return undefined;
  })();
  return part?.text ? part : undefined;
}
