// The person's ctrl+o in the face (docs/face.md, "Unfolding"): the shell
// keeps it, so a face reloaded mid-way opens as the person left it.
import { afterEach, expect, test } from "bun:test";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { FaceHost, type FaceState } from "./host.ts";

const worker = new URL("../../../face/src/worker.ts", import.meta.url).pathname;
const ends: (() => void)[] = [];

afterEach(() => {
  for (const end of ends.splice(0)) end();
});

const output = Array.from({ length: 12 }, (_, i) => `line ${i + 1}`).join("\n");

const probe = { id: "c", kind: "tool", status: "done", name: "Probe", text: "{}", output };

// A wrapper's mirror socket of the items given, by default one call whose
// output the face cuts, served to the face at url.
function session(items: object[] = [probe], url = worker) {
  const dir = mkdtempSync(join(tmpdir(), "face-"));
  ends.push(() => rmSync(dir, { recursive: true, force: true }));
  const path = join(dir, "m.sock");
  const lines = [
    { op: "hello", v: 1, inputs: [] },
    { op: "begin", items: items.length, dropped: 0, turn: "idle" },
    ...items.map((item) => ({ op: "item", item })),
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
  const host = new FaceHost({ path, url, onState: () => {} });
  ends.push(() => host.end());
  const until = async (ok: (s: FaceState) => boolean, within = 3000) => {
    const deadline = Date.now() + within;
    while (Date.now() < deadline) {
      if (ok(host.state)) return host.state;
      await Bun.sleep(10);
    }
    throw new Error(`no such state: ${JSON.stringify(host.state)}`);
  };
  return { host, until, dir };
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
  await until((s) => has(s, "     … +9 lines (ctrl+o to expand)"));
  host.unfold();
  expect(host.state.expanded).toBe(true);
  await until((s) => has(s, "     line 12"));
  host.start();
  await until((s) => has(s, "     line 12"));
  host.unfold();
  expect(host.state.expanded).toBe(false);
  await until((s) => has(s, "     … +9 lines (ctrl+o to expand)"));
});

// Review of 288cd7f: a selection is held by the rows' places, so rows opened
// above it moved its light onto a call; whatever opens or folds drops it, as
// a press or a key does.
test("ctrl+o, opening or folding, drops the selection; a click's press does already", async () => {
  const read = { ...probe, id: "r", name: "Read", text: JSON.stringify({ file_path: "/a" }) };
  const reply = { id: "t", kind: "text", status: "settled", text: "SELECT me" };
  const { host, until } = session([read, reply]);
  host.size(40, 30);
  host.start();
  const shown = await until((s) => has(s, "● SELECT me"));
  const row = shown.lines.findIndex((l) => l.spans.some((s) => s.text.includes("SELECT")));
  const selected = (s: FaceState) => s.lines.some((l) => l.spans.some((sp) => sp.selected));
  host.mouse("down", 2, row, true);
  host.mouse("drag", 8, row, true);
  host.mouse("up", 8, row, true);
  await until(selected);
  host.unfold();
  await until((s) => has(s, "● Read(/a)"));
  expect(selected(host.state)).toBe(false);
  host.mouse("down", 2, row, true);
  host.mouse("drag", 8, row, true);
  host.mouse("up", 8, row, true);
  await until(selected);
  host.unfold();
  await until((s) => !has(s, "● Read(/a)"));
  expect(selected(host.state)).toBe(false);
});

// Review of 288cd7f: a face refused for its contract stays refused; what the
// shell sends after start, its ctrl+o among it, leaves the last good frame
// and the error.
test("a face of another contract, reloaded opened, stays refused", async () => {
  const dir = mkdtempSync(join(tmpdir(), "face-"));
  ends.push(() => rmSync(dir, { recursive: true, force: true }));
  const url = join(dir, "face.ts");
  writeFileSync(url, `import ${JSON.stringify(worker)};\n`);
  const { host, until } = session([probe], url);
  host.size(40, 30);
  host.start();
  await until((s) => has(s, "     … +9 lines (ctrl+o to expand)"));
  host.unfold();
  const good = await until((s) => has(s, "     line 12"));
  // The real face, told by its shell of a contract one past its own.
  writeFileSync(
    url,
    `import ${JSON.stringify(worker)};
const real = self.onmessage;
self.onmessage = (e) => real({ data: e.data.kind === "start" ? { ...e.data, contract: e.data.contract + 1 } : e.data });\n`,
  );
  host.start();
  await until((s) => s.error !== undefined);
  host.size(41, 30);
  host.scroll(1);
  await Bun.sleep(300);
  expect(host.state.error).toMatch(/^this face speaks contract \d+, the shell \d+$/);
  expect(host.state.lines).toEqual(good.lines);
});
