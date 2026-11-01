// The person's Esc and shift+tab cross the face's worker to the session
// (docs/face.md, "The face"), and a refusal comes back said under the input;
// when each reaches the session, and the level the rule is told, is the
// face's glue, tested in process in face/src/link.test.ts.
import { afterEach, expect, test } from "bun:test";
import type { FaceState } from "./host.ts";
import { cleanup, hosted, mirror, scratch, worker } from "./hostkit.ts";

afterEach(cleanup);

const drawn = (s: FaceState) => s.link === "live" && s.lines.some((l) => l.spans.length > 0);

test("Esc reaches a working session through the worker, and its refusal is said", async () => {
  const wrapper = mirror(scratch(), {
    replies: ["a reply"],
    begin: { turn: "working" },
    inputs: ["prompt", "interrupt"],
    refuse: "pi is not working",
  });
  const { host, until } = hosted(worker, wrapper.path);
  host.start();
  await until(drawn);
  host.escape();
  const said = await until((s) => s.said !== undefined);
  expect(wrapper.heard).toMatchObject([{ kind: "interrupt", text: "" }]);
  expect(said.said).toBe("not interrupted: pi is not working");
});

test("shift+tab reaches the session through the worker, whose level the rule is told, and its refusal is said", async () => {
  const wrapper = mirror(scratch(), {
    begin: { level: "high" },
    inputs: ["prompt", "thinking"],
    refuse: "Current model does not support thinking",
  });
  const { host, until } = hosted(worker, wrapper.path);
  host.start();
  expect((await until(drawn)).level).toBe("high");
  host.thinking();
  const said = await until((s) => s.said !== undefined);
  expect(wrapper.heard).toMatchObject([{ kind: "thinking", text: "" }]);
  expect(said.said).toBe("not changed: Current model does not support thinking");
});
