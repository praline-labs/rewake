// The person's Esc in the face (docs/face.md, "The face"): it drops a
// selection first; else it interrupts the session's working turn, where the
// session takes an interrupt, as the harness's own Esc does; an idle session,
// or one that takes none, hears nothing.
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

// A wrapper's mirror socket of one reply in the turn given, taking the inputs
// given: it keeps what the face writes and answers an interrupt as told.
function session(turn: string, inputs: string[], refuse?: string) {
  const dir = mkdtempSync(join(tmpdir(), "face-"));
  ends.push(() => rmSync(dir, { recursive: true, force: true }));
  const path = join(dir, "m.sock");
  const item = { id: "r", kind: "text", status: "settled", text: "a reply to select" };
  const lines = [
    { op: "hello", v: 1, inputs },
    { op: "begin", items: 1, dropped: 0, turn },
    { op: "item", item },
    { op: "end" },
  ];
  const heard: { id: string; kind: string; text: string }[] = [];
  const server = Bun.listen({
    unix: path,
    socket: {
      open(s) {
        s.write(lines.map((l) => `${JSON.stringify(l)}\n`).join(""));
      },
      data(s, chunk) {
        for (const raw of chunk.toString().split("\n").filter(Boolean)) {
          const input = JSON.parse(raw);
          heard.push(input);
          const answer = refuse
            ? { op: "answer", id: input.id, ok: false, detail: refuse }
            : { op: "answer", id: input.id, ok: true };
          s.write(`${JSON.stringify(answer)}\n`);
        }
      },
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
  return { host, heard, until };
}

const drawn = (s: FaceState) => s.link === "live" && s.lines.some((l) => l.spans.length > 0);

test("Esc interrupts a working session that takes it, and says a refusal", async () => {
  const { host, heard, until } = session("working", ["prompt", "interrupt"]);
  host.start();
  await until(drawn);
  host.escape();
  await until(() => heard.length === 1);
  expect(heard[0]).toMatchObject({ kind: "interrupt", text: "" });
  expect(host.state.said).toBeUndefined();

  const refused = session("working", ["prompt", "interrupt"], "pi is not working");
  refused.host.start();
  await refused.until(drawn);
  refused.host.escape();
  const said = await refused.until((s) => s.said !== undefined);
  expect(said.said).toBe("not interrupted: pi is not working");
});

test("Esc while idle, or to a session that takes no interrupt, sends nothing", async () => {
  for (const [turn, inputs] of [
    ["idle", ["prompt", "interrupt"]],
    ["working", ["prompt"]],
  ] as const) {
    const { host, heard, until } = session(turn, [...inputs]);
    host.start();
    await until(drawn);
    host.escape();
    // A prompt after it is heard, so the Esc went before and sent nothing.
    host.prompt("next");
    await until(() => heard.length > 0);
    expect(heard.map((h) => h.kind)).toEqual(["prompt"]);
  }
});

test("Esc drops a selection first, and only the next one interrupts", async () => {
  const { host, heard, until } = session("working", ["prompt", "interrupt"]);
  host.start();
  const shown = await until(drawn);
  const row = shown.lines.findIndex((l) => l.spans.some((s) => s.text.includes("reply")));
  host.mouse("down", 2, row, true);
  host.mouse("drag", 9, row, true);
  await until((s) => s.lines[row]?.spans.some((sp) => sp.selected) ?? false);
  host.escape();
  await until((s) => !s.lines[row]?.spans.some((sp) => sp.selected));
  host.escape();
  host.prompt("next");
  await until(() => heard.length === 2);
  expect(heard.map((h) => h.kind)).toEqual(["interrupt", "prompt"]);
});
