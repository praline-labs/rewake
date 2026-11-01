// The face's tool vocabulary through its one interface: each row of the table
// an item in, all the face draws of the call out.
import { expect, test } from "bun:test";
import { cases, type Described, described } from "./call.cases.ts";
import { type Call, callOf } from "./call.ts";
import type { Item } from "./mirror.gen.ts";

const itemOf = (tool: string, input: Record<string, unknown> | string): Item => ({
  id: "c",
  kind: "tool",
  status: "done",
  name: tool,
  text: typeof input === "string" ? input : JSON.stringify(input),
});

for (const c of cases)
  test(`a call: ${c.what}`, () => {
    const call = callOf(itemOf(c.tool, c.input));
    const { hidden, name, argument, op, copy, result, permitRow } = call;
    const said: Described = { hidden, name, argument, op, copy, result, permitRow };
    expect(said).toEqual(described(c));
    // The rest the row says, a field it says is none included.
    for (const [key, value] of Object.entries(c.call))
      expect([key, call[key as keyof Call]]).toEqual([key, value]);
  });

test("a call is read again once its input grows", () => {
  const item = itemOf("Bash", '{"command":"l');
  expect(callOf(item).op).toBe("command");
  item.text = '{"command":"ls"}';
  expect(callOf(item).op).toBe("list");
});
