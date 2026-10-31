// pi's tools in the face's words (docs/face.md, "The look"; research-pi.md):
// the face draws every harness as Claude Code's terminal looks and knows no
// harness, so its adapter names each of pi's built-in tools by its Claude Code
// kin and gives its input in that tool's fields. A tool with no kin keeps its
// own name and input, which the face draws by its generic rule.

type Args = Record<string, unknown>;

// faced is a call as the face reads it: its name and its input.
export type Faced = { name: string; input: Args };

const given = (args: Args, keys: Record<string, string>): Args => {
  const out: Args = {};
  for (const [from, to] of Object.entries(keys)) if (args[from] !== undefined) out[to] = args[from];
  return out;
};

// quoted is a path as a shell reads it, quoted only when it must be.
const quoted = (path: string) =>
  /^[\w./~@%+=:,-]+$/.test(path) ? path : `'${path.replaceAll("'", `'\\''`)}'`;

type Edit = { oldText?: unknown; newText?: unknown };

export function facedTool(name: string, raw: unknown): Faced {
  const args: Args = raw && typeof raw === "object" && !Array.isArray(raw) ? (raw as Args) : {};
  switch (name) {
    case "bash":
      return { name: "Bash", input: given(args, { command: "command" }) };
    case "read":
      return {
        name: "Read",
        input: given(args, { path: "file_path", offset: "offset", limit: "limit" }),
      };
    case "write":
      return { name: "Write", input: given(args, { path: "file_path", content: "content" }) };
    case "edit": {
      const edits = (Array.isArray(args.edits) ? args.edits : []).map((e: Edit) => ({
        old_string: typeof e?.oldText === "string" ? e.oldText : "",
        new_string: typeof e?.newText === "string" ? e.newText : "",
      }));
      const input = given(args, { path: "file_path" });
      // One edit is Claude Code's Edit; several, the edits of its MultiEdit.
      if (edits.length === 1) return { name: "Edit", input: { ...input, ...edits[0] } };
      return { name: "MultiEdit", input: { ...input, edits } };
    }
    case "grep":
      return {
        name: "Grep",
        input: given(args, {
          pattern: "pattern",
          path: "path",
          glob: "glob",
          ignoreCase: "-i",
          context: "-C",
          limit: "head_limit",
        }),
      };
    case "find":
      return { name: "Glob", input: given(args, { pattern: "pattern", path: "path" }) };
    case "ls": {
      // Claude Code lists a directory with its shell, so the face counts it
      // as a listed directory.
      const path = typeof args.path === "string" && args.path !== "" ? ` ${quoted(args.path)}` : "";
      return { name: "Bash", input: { command: `ls${path}` } };
    }
    default:
      return { name, input: args };
  }
}

// resultText is a tool's result as the face draws its output: the text of
// its content, an image named as one.
export function resultText(result: unknown): string {
  const content = (result as { content?: unknown })?.content;
  if (!Array.isArray(content)) return typeof result === "string" ? result : "";
  return content
    .map((part: { type?: string; text?: unknown }) =>
      part?.type === "text" && typeof part.text === "string"
        ? part.text
        : part?.type === "image"
          ? "[image]"
          : "",
    )
    .filter((text) => text !== "")
    .join("\n");
}
