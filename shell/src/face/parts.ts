// The parts a click copies whole (docs/face.md, "Copying"): a reply as its
// markdown source, a code block's code, a call's input — a command, a path,
// a pattern, else the input as given — a run's inputs one a line, a call's
// output, an edit's old or new lines, a written file's lines. A row names its
// part by a key (select.ts); the text is taken from the model when the click
// comes, so the rows carry no copy of it.
import { entriesOf } from "./fold.ts";
import { codeBlocks } from "./markdown.ts";
import type { Item } from "./mirror.gen.ts";
import { field, inputOf } from "./tools.ts";

// The field a call's head shows of its input, the one a click copies.
const shownField: Record<string, string> = {
  Bash: "command",
  Read: "file_path",
  Edit: "file_path",
  MultiEdit: "file_path",
  Write: "file_path",
  NotebookEdit: "notebook_path",
  WebFetch: "url",
  WebSearch: "query",
  Skill: "skill",
  Grep: "pattern",
  Glob: "pattern",
};

function inputText(item: Item): string {
  const input = inputOf(item);
  const shown = field(input, shownField[item.name ?? ""] ?? "");
  if (shown !== undefined) return shown;
  return input ? JSON.stringify(input, null, 2) : item.text;
}

// edits is an edit's old or new strings, one after another.
function edits(item: Item, key: "old_string" | "new_string"): string {
  const input = inputOf(item);
  const all: unknown[] = Array.isArray(input?.edits) ? input.edits : [input];
  return all
    .map((e) => field(e as Record<string, unknown> | undefined, key))
    .filter((s): s is string => s !== undefined)
    .join("\n");
}

// partOf is the text of the part a key names and what to call it, none when
// its item is gone or the part is empty.
export function partOf(items: Item[], key: string): { text: string; what: string } | undefined {
  const at = key.indexOf(" ");
  const kind = key.slice(0, at);
  const id = key.slice(at + 1);
  const item = items.find((i) => i.id === id);
  if (!item) return undefined;
  if (kind === "run") {
    // A run is named by its first call: its calls' inputs, one a line.
    const run = entriesOf(items, false).find((e) => "calls" in e && e.calls[0]?.id === id);
    const calls = run && "calls" in run ? run.calls : [item];
    const one = calls.length === 1 && item.name === "Bash";
    const text = calls.map(inputText).join("\n");
    return text ? { text, what: one ? "the command" : "the run's inputs" } : undefined;
  }
  const part = ((): { text: string; what: string } | undefined => {
    const code = /^code(\d+)$/.exec(kind);
    if (code) return { text: codeBlocks(item.text)[Number(code[1])] ?? "", what: "the code block" };
    switch (kind) {
      case "reply":
        return { text: item.text, what: "the reply" };
      case "input":
        return {
          text: inputText(item),
          what: item.name === "Bash" ? "the command" : "the call's input",
        };
      case "output":
        return { text: item.output ?? "", what: "the call's output" };
      case "old":
        return { text: edits(item, "old_string"), what: "the old lines" };
      case "new":
        return { text: edits(item, "new_string"), what: "the new lines" };
      case "content":
        return { text: field(inputOf(item), "content") ?? "", what: "the file's lines" };
    }
    return undefined;
  })();
  return part?.text ? part : undefined;
}
