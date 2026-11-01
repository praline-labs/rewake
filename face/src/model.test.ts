// The face's copy of the mirror keeps what the input's rule shows: the
// session's mode and thinking level, said on a snapshot's begin and changed
// by their own lines.
import { expect, test } from "bun:test";
import { bounds } from "./mirror.gen.ts";
import { FaceModel } from "./model.ts";

test("the model keeps the session's mode and level from begin and from their lines", () => {
  const model = new FaceModel();
  model.apply({ op: "hello", v: bounds.version, inputs: ["thinking"] });
  model.apply({ op: "begin", items: 0, turn: "idle", level: "high" });
  model.apply({ op: "end" });
  expect([model.mode, model.level]).toEqual([undefined, "high"]);
  expect(model.apply({ op: "level", level: "xhigh" })).toBe(true);
  expect(model.apply({ op: "mode", mode: "plan" })).toBe(true);
  expect([model.mode, model.level]).toEqual(["plan", "xhigh"]);
  // A fresh snapshot of a session that has said neither.
  model.apply({ op: "begin", items: 0, turn: "idle" });
  expect([model.mode, model.level]).toEqual([undefined, undefined]);
});
