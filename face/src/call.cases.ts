// The face's tool vocabulary as a table (call.ts): a call's name and input,
// and all the face draws of it. call.test.ts runs every row through callOf,
// and an adapter that renames its harness's tools into this vocabulary tests
// that each form it makes is a row here, so what the face reads of a call is
// written down in one place.

import type { Call } from "./call.ts";

const home = process.env.HOME ?? "/home/someone";

// Every row says these of its call; the rest of Call where it matters.
export type Described = Pick<
  Call,
  "hidden" | "name" | "argument" | "op" | "copy" | "result" | "permitRow"
>;

export type Case = {
  what: string;
  // tool and input are the call as its adapter names and gives it; an input
  // that is a string is the text as it came, which may not be JSON.
  tool: string;
  input: Record<string, unknown> | string;
  call: Partial<Call>;
};

// described is what a row says of the seven every call has, what it leaves
// unsaid as most calls are: drawn, folding into no run, its output under it,
// a permit row when allowed.
export function described(c: Case): Described {
  const all = {
    hidden: false,
    op: undefined,
    result: "output" as const,
    permitRow: true,
    name: c.tool,
    argument: "",
    copy: { text: typeof c.input === "string" ? c.input : "", what: "the call's input" },
    ...c.call,
  };
  const { hidden, name, argument, op, copy, result, permitRow } = all;
  return { hidden, name, argument, op, copy, result, permitRow };
}

const command = (text: string) => ({ text, what: "the command" });
const given = (text: string) => ({ text, what: "the call's input" });
const long = `make ${"x".repeat(200)}`;

export const cases: Case[] = [
  {
    what: "a command, its description shown while it runs",
    tool: "Bash",
    input: { command: "make all", description: "Build" },
    call: {
      argument: "make all",
      op: "command",
      copy: command("make all"),
      hint: "$ make all",
      shell: true,
      description: "Build",
      empty: "(No output)",
    },
  },
  {
    what: "a command and nothing else",
    tool: "Bash",
    input: { command: "make" },
    call: { argument: "make", op: "command", copy: command("make"), description: undefined },
  },
  {
    what: "a listing",
    tool: "Bash",
    input: { command: "ls" },
    call: { argument: "ls", op: "list", copy: command("ls"), hint: "$ ls" },
  },
  {
    what: "a listing of a directory",
    tool: "Bash",
    input: { command: "ls src" },
    call: { argument: "ls src", op: "list", copy: command("ls src"), hint: "$ ls src" },
  },
  {
    what: "a listing of a directory the shell must have quoted",
    tool: "Bash",
    input: { command: "ls 'my dir'" },
    call: { argument: "ls 'my dir'", op: "list", copy: command("ls 'my dir'") },
  },
  {
    what: "a pipeline of reads",
    tool: "Bash",
    input: { command: "cat a | wc -l" },
    call: { argument: "cat a | wc -l", op: "read", copy: command("cat a | wc -l") },
  },
  {
    what: "a search behind a variable",
    tool: "Bash",
    input: { command: "LC_ALL=C grep -rl x ." },
    call: {
      argument: "LC_ALL=C grep -rl x .",
      op: "search",
      copy: command("LC_ALL=C grep -rl x ."),
    },
  },
  {
    what: "a chain with a word of no set, a shell command",
    tool: "Bash",
    input: { command: "cd /a && grep -rl x ." },
    call: {
      argument: "cd /a && grep -rl x .",
      op: "command",
      copy: command("cd /a && grep -rl x ."),
    },
  },
  {
    what: "a long command, its head cut, whole when copied",
    tool: "Bash",
    input: { command: `${long}\nline two\nline three` },
    call: {
      argument: `make ${"x".repeat(155)}…`,
      op: "command",
      copy: command(`${long}\nline two\nline three`),
    },
  },
  {
    what: "a command cut to its bound, no longer JSON",
    tool: "Bash",
    input: '{"command":"l',
    call: { argument: '{"command":"l', op: "command", copy: command('{"command":"l'), hint: "$ " },
  },
  {
    what: "a read",
    tool: "Read",
    input: { file_path: "/a.ts" },
    call: {
      argument: "/a.ts",
      op: "read",
      file: "/a.ts",
      hint: "/a.ts",
      copy: given("/a.ts"),
      empty: "(No content)",
    },
  },
  {
    what: "a read from a line",
    tool: "Read",
    input: { file_path: "a.go", offset: 2 },
    call: { argument: "a.go", op: "read", file: "a.go", copy: given("a.go") },
  },
  {
    what: "a read of pages, under the home directory",
    tool: "Read",
    input: { file_path: `${home}/a.pdf`, pages: "1-3" },
    call: {
      argument: "~/a.pdf · pages 1-3",
      op: "read",
      file: `${home}/a.pdf`,
      hint: "~/a.pdf",
      copy: given(`${home}/a.pdf`),
    },
  },
  {
    what: "an edit",
    tool: "Edit",
    input: { file_path: "a", old_string: "x", new_string: "y" },
    call: {
      name: "Update",
      argument: "a",
      result: "edit",
      copy: given("a"),
      edits: [{ old: "x", new: "y" }],
    },
  },
  {
    what: "an edit from nothing",
    tool: "Edit",
    input: { file_path: "/new.ts", old_string: "", new_string: "a" },
    call: { name: "Create", argument: "/new.ts", result: "edit", copy: given("/new.ts") },
  },
  {
    what: "several edits",
    tool: "MultiEdit",
    input: {
      file_path: "a",
      edits: [
        { old_string: "x", new_string: "y" },
        { old_string: "p", new_string: "q" },
      ],
    },
    call: {
      name: "Update",
      argument: "a",
      result: "edit",
      copy: given("a"),
      edits: [
        { old: "x", new: "y" },
        { old: "p", new: "q" },
      ],
    },
  },
  {
    what: "edits still streaming, none yet",
    tool: "MultiEdit",
    input: { edits: [] },
    call: { name: "Update", result: "edit", copy: given('{\n  "edits": []\n}'), edits: [] },
  },
  {
    what: "an edit whose input is not JSON",
    tool: "Edit",
    input: '{"file_path":"/a',
    call: { name: "Update", argument: '{"file_path":"/a', result: "edit", edits: undefined },
  },
  {
    what: "a write",
    tool: "Write",
    input: { file_path: "a", content: "x" },
    call: {
      argument: "a",
      result: "write",
      copy: given("a"),
      written: { content: "x", path: "a" },
    },
  },
  {
    what: "a write whose input is not JSON",
    tool: "Write",
    input: '{"file_path":"/a',
    call: { argument: '{"file_path":"/a', result: "write", written: undefined },
  },
  {
    what: "a search by content",
    tool: "Grep",
    input: { pattern: "x", path: "src", "-i": true },
    call: {
      name: "Search",
      argument: 'pattern: "x", path: "src"',
      op: "search",
      hint: '"x"',
      copy: given("x"),
    },
  },
  {
    what: "a search by name",
    tool: "Glob",
    input: { pattern: "*.ts", path: "src" },
    call: {
      name: "Search",
      argument: 'pattern: "*.ts", path: "src"',
      op: "search",
      copy: given("*.ts"),
    },
  },
  {
    what: "a search with no path",
    tool: "Grep",
    input: { pattern: "x" },
    call: { name: "Search", argument: 'pattern: "x"', op: "search", copy: given("x") },
  },
  {
    what: "a notebook's edit",
    tool: "NotebookEdit",
    input: { notebook_path: "/n.ipynb", new_source: "x" },
    call: { argument: "/n.ipynb", copy: given("/n.ipynb") },
  },
  {
    what: "a fetch",
    tool: "WebFetch",
    input: { url: "https://example.com", prompt: "p" },
    call: { name: "Fetch", argument: "https://example.com", copy: given("https://example.com") },
  },
  {
    what: "a web search",
    tool: "WebSearch",
    input: { query: "q" },
    call: { name: "Web Search", argument: '"q"', copy: given("q") },
  },
  {
    what: "a subagent, no permit row of its own",
    tool: "Agent",
    input: { description: "Look\naround", prompt: "p" },
    call: {
      argument: "Look around",
      permitRow: false,
      copy: given('{\n  "description": "Look\\naround",\n  "prompt": "p"\n}'),
    },
  },
  {
    what: "a subagent by its older name",
    tool: "Task",
    input: { description: "Look" },
    call: { argument: "Look", copy: given('{\n  "description": "Look"\n}') },
  },
  {
    what: "a skill",
    tool: "Skill",
    input: { skill: "mods" },
    call: { argument: "mods", copy: given("mods") },
  },
  {
    what: "a tool search, drawn nowhere",
    tool: "ToolSearch",
    input: { query: "q" },
    call: { hidden: true, argument: 'query: "q"', copy: given('{\n  "query": "q"\n}') },
  },
  {
    what: "a to-do list, drawn as its own panel",
    tool: "TodoWrite",
    input: { todos: [] },
    call: { hidden: true, argument: "todos: []", copy: given('{\n  "todos": []\n}') },
  },
  {
    what: "a server's tool",
    tool: "mcp__rewake__probe",
    input: { n: 1 },
    call: { name: "rewake - probe (MCP)", argument: "n: 1", copy: given('{\n  "n": 1\n}') },
  },
  {
    what: "a tool of no words of its own",
    tool: "brief_tick",
    input: { n: 1 },
    call: { argument: "n: 1", copy: given('{\n  "n": 1\n}') },
  },
  {
    what: "a call with no input yet",
    tool: "Probe",
    input: "",
    call: { argument: "", copy: given("") },
  },
];
