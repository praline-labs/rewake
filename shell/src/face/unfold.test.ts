// The person's ctrl+o in the face (docs/face.md, "Unfolding"): the shell
// keeps it, so a face reloaded mid-way opens as the person left it. What
// ctrl+o does inside the face is tested in process in face/src/face.test.ts.
import { afterEach, expect, test } from "bun:test";
import type { FaceState } from "./host.ts";
import { cleanup, hosted, mirror, scratch, worker } from "./hostkit.ts";

afterEach(cleanup);

const output = Array.from({ length: 12 }, (_, i) => `line ${i + 1}`).join("\n");
const probe = { id: "c", kind: "tool", status: "done", name: "Probe", text: "{}", output };

const has = (s: FaceState, text: string) =>
  s.link === "live" &&
  s.lines.some(
    (l) =>
      l.spans
        .map((sp) => sp.text)
        .join("")
        .trimEnd() === text,
  );

test("ctrl+o opens everything, and a reloaded face opens it again", async () => {
  const { host, until } = hosted(worker, mirror(scratch(), { items: [probe], inputs: [] }).path);
  host.size(40, 30);
  host.start();
  await until((s) => has(s, "     … +9 lines (ctrl+o to expand)"));
  host.unfold();
  expect(host.state.expanded).toBe(true);
  await until((s) => has(s, "     line 12"));
  host.reload();
  await until((s) => has(s, "     line 12"));
  host.unfold();
  expect(host.state.expanded).toBe(false);
  await until((s) => has(s, "     … +9 lines (ctrl+o to expand)"));
});
