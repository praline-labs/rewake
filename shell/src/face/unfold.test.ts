// The person's ctrl+o in the face (docs/face.md, "Unfolding"): the shell
// keeps it, so a face reloaded mid-way opens as the person left it.
import { afterEach, expect, test } from "bun:test";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { FaceHost, type FaceState } from "./host.ts";

const worker = new URL("../../../face/src/worker.ts", import.meta.url).pathname;
const ends: (() => void)[] = [];

afterEach(() => {
  for (const end of ends.splice(0)) end();
});

const output = Array.from({ length: 12 }, (_, i) => `line ${i + 1}`).join("\n");

// A wrapper's mirror socket of one call whose output the face cuts.
function session() {
  const dir = mkdtempSync(join(tmpdir(), "face-"));
  ends.push(() => rmSync(dir, { recursive: true, force: true }));
  const path = join(dir, "m.sock");
  const item = { id: "c", kind: "tool", status: "done", name: "Probe", text: "{}", output };
  const lines = [
    { op: "hello", v: 1, inputs: [] },
    { op: "begin", items: 1, dropped: 0, turn: "idle" },
    { op: "item", item },
    { op: "end" },
  ];
  const server = Bun.listen({
    unix: path,
    socket: {
      open(s) {
        s.write(lines.map((l) => `${JSON.stringify(l)}\n`).join(""));
      },
      data() {},
    },
  });
  ends.push(() => server.stop(true));
  const host = new FaceHost({ path, url: worker, onState: () => {} });
  ends.push(() => host.end());
  const until = async (ok: (s: FaceState) => boolean, within = 3000) => {
    const deadline = Date.now() + within;
    while (Date.now() < deadline) {
      if (ok(host.state)) return host.state;
      await Bun.sleep(10);
    }
    throw new Error(`no such state: ${JSON.stringify(host.state)}`);
  };
  return { host, until };
}

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
  const { host, until } = session();
  host.size(40, 30);
  host.start();
  await until((s) => has(s, "     … +9 lines"));
  host.unfold();
  expect(host.state.expanded).toBe(true);
  await until((s) => has(s, "     line 12"));
  host.start();
  await until((s) => has(s, "     line 12"));
  host.unfold();
  expect(host.state.expanded).toBe(false);
  await until((s) => has(s, "     … +9 lines"));
});
