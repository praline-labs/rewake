// The native editor's keys (editor.ts): Shift+Tab is not a work-mode cycle
// but pi's own thinking key, Alt+T is taken beside it, and a bracketed paste
// stays whole for the native editor.
import { expect, mock, test } from "bun:test";

const delegated: string[] = [];

mock.module("@earendil-works/pi-coding-agent", () => ({
  CustomEditor: class {
    handleInput(data: string) {
      delegated.push(data);
    }
  },
}));

mock.module("@earendil-works/pi-tui", () => ({
  matchesKey: (data: string, key: string) =>
    (key === "shift+tab" && data === "\x1b[Z") || (key === "alt+t" && data === "\x1bt"),
}));

const { modeEditor } = await import("./editor.ts");

function rig() {
  const hooks = new Map<string, (event: unknown, ctx: unknown) => unknown>();
  const cycles: string[] = [];
  const thoughts: string[] = [];
  const pi = {
    on: (name: string, hook: (event: unknown, ctx: unknown) => unknown) => hooks.set(name, hook),
  };
  const modes = {
    cycle: () => {
      cycles.push("cycle");
      return Promise.resolve({ ok: true });
    },
    thinking: () => {
      thoughts.push("thinking");
      return Promise.resolve({ ok: true });
    },
  };
  modeEditor(pi as never, modes as never);
  let factory:
    | ((tui: unknown, theme: unknown, keys: unknown) => { handleInput(s: string): void })
    | undefined;
  const ui = { setEditorComponent: (next: typeof factory) => (factory = next) };
  hooks.get("session_start")?.({}, { mode: "tui", ui });
  if (!factory) throw new Error("the editor was not registered");
  const editor = factory({}, {}, {});
  return { editor, cycles, thoughts };
}

test("Shift+Tab stays native and is never a work-mode cycle", () => {
  const r = rig();
  delegated.length = 0;
  r.editor.handleInput("\x1b[Z");
  expect(delegated).toEqual(["\x1b[Z"]);
  expect(r.cycles).toEqual([]);
  expect(r.thoughts).toEqual([]);
});

test("Alt+T is the thinking key, and a bracketed paste is left whole", () => {
  const r = rig();
  delegated.length = 0;
  r.editor.handleInput("\x1bt");
  expect(r.thoughts).toEqual(["thinking"]);
  expect(delegated).toEqual([]);

  r.editor.handleInput("\x1b[200~pasted");
  r.editor.handleInput("\x1bt");
  expect(r.thoughts).toEqual(["thinking"]);
  expect(delegated).toEqual(["\x1b[200~pasted", "\x1bt"]);
  r.editor.handleInput("\x1b[201~");
  r.editor.handleInput("\x1bt");
  expect(r.thoughts).toEqual(["thinking", "thinking"]);
});
