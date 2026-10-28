// A face that fails to load, throws or hangs costs its own drawing alone: the
// shell's thread hears it, keeps the last lines and goes on (docs/face.md,
// "Hot reload").
import { afterEach, expect, test } from "bun:test";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { FaceHost, type FaceState } from "./host.ts";

const worker = new URL("./worker.ts", import.meta.url).pathname;
const dirs: string[] = [];
const ends: (() => void)[] = [];

afterEach(() => {
  for (const end of ends.splice(0)) end();
  for (const dir of dirs.splice(0)) rmSync(dir, { recursive: true, force: true });
});

function scratch() {
  const dir = mkdtempSync(join(tmpdir(), "face-"));
  dirs.push(dir);
  return dir;
}

// A face's module in a directory of its own, rewritten as a reload would meet
// a change of the sources.
function faceFile(dir: string, source: string) {
  const path = join(dir, "face.ts");
  writeFileSync(path, source);
  return path;
}

function hosted(url: string, path = "/nowhere.sock", timing = {}) {
  const states: FaceState[] = [];
  const host = new FaceHost({ path, url, onState: (s) => states.push(s), ...timing });
  ends.push(() => host.end());
  const until = async (ok: (s: FaceState) => boolean, within = 3000) => {
    const deadline = Date.now() + within;
    while (Date.now() < deadline) {
      if (ok(host.state)) return host.state;
      await Bun.sleep(10);
    }
    throw new Error(`no such state: ${JSON.stringify(host.state)}`);
  };
  return { host, states, until };
}

// A wrapper's mirror socket serving one snapshot of one prompt.
function mirror(dir: string, text: string) {
  const path = join(dir, "m.sock");
  const lines = [
    { op: "hello", v: 1, inputs: ["prompt"] },
    { op: "begin", items: 1, dropped: 0, turn: "idle" },
    { op: "item", item: { id: "p", kind: "prompt", status: "settled", text } },
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
  return path;
}

test("a face draws the wrapper's snapshot, and a reload that fails to import keeps its lines with the error", async () => {
  const dir = scratch();
  const url = faceFile(dir, `import ${JSON.stringify(worker)};\n`);
  const { host, until } = hosted(url, mirror(dir, "say hi"));
  host.start();
  const drawn = await until((s) => s.link === "live" && s.lines.length > 0);
  // Every row is filled to the pane, so the layer is opaque.
  expect(drawn.lines.length).toBe(24);
  expect(drawn.lines.map((l) => l.text.trimEnd()).filter(Boolean)).toEqual(["› say hi"]);

  faceFile(dir, `import "./missing.ts";\n`);
  host.start();
  const failed = await until((s) => s.link === "stopped");
  expect(failed.error).toContain("missing");
  expect(failed.lines).toEqual(drawn.lines);
  expect(host.running).toBe(false);

  // Fixed again, the next reload draws afresh from the snapshot.
  faceFile(dir, `import ${JSON.stringify(worker)};\n`);
  host.start();
  await until((s) => s.link === "live" && s.error === undefined);
});

test("a face that throws, as it loads or on a message, is stopped with its error shown", async () => {
  for (const source of [
    `throw new Error("broken at load");\n`,
    `self.onmessage = () => { throw new Error("broken on start"); };\n`,
  ]) {
    const { host, until } = hosted(faceFile(scratch(), source));
    host.start();
    const failed = await until((s) => s.link === "stopped");
    expect(failed.error).toContain("broken");
  }
});

test("a face that hangs is stopped when it misses its ping, and the shell's thread runs on", async () => {
  const url = faceFile(scratch(), `self.onmessage = () => { while (true) {} };\n`);
  const { host, until } = hosted(url, "/nowhere.sock", { pingEvery: 50, pongWithin: 200 });
  let ticks = 0;
  const tick = setInterval(() => ticks++, 20);
  host.start();
  const failed = await until((s) => s.link === "stopped");
  clearInterval(tick);
  expect(failed.error).toContain("did not answer");
  expect(ticks > 5).toBe(true);
});

// A wrapper's mirror socket that serves a snapshot and never answers a
// prompt, its connections at hand to cut.
function silent(dir: string) {
  const path = join(dir, "s.sock");
  const open: { end(): void }[] = [];
  const lines = [
    { op: "hello", v: 1, inputs: ["prompt"] },
    { op: "begin", items: 0, dropped: 0, turn: "idle" },
    { op: "end" },
  ];
  const server = Bun.listen({
    unix: path,
    socket: {
      open(s) {
        open.push(s);
        s.write(lines.map((l) => `${JSON.stringify(l)}\n`).join(""));
      },
      data() {},
    },
  });
  ends.push(() => server.stop(true));
  return { path, cut: () => open.at(-1)?.end() };
}

test("prompts that get no answer are bounded, expire, and go with their link, a reload and the face's end", async () => {
  const dir = scratch();
  const wrapper = silent(dir);
  const url = faceFile(dir, `import ${JSON.stringify(worker)};\n`);
  const { host, until } = hosted(url, wrapper.path, { pingEvery: 50, promptsExpireAfter: 500 });
  const sent = (host as unknown as { sent: Map<string, { text: string }> }).sent;
  const waiting = () => [...sent.values()];
  const live = () => until((s) => s.link === "live");
  host.start();
  await live();

  // Three disconnects, each with prompts in flight: none outlives its link.
  for (let n = 1; n <= 3; n++) {
    for (let i = 0; i < n; i++) host.prompt(`lost ${n}.${i}`);
    expect(waiting().length).toBe(n);
    wrapper.cut();
    await until((s) => s.link === "lost");
    expect(waiting().length).toBe(0);
    await live();
  }

  // Many large prompts: a few wait, each with what a refusal shows of it.
  for (let i = 0; i < 200; i++) host.prompt("x".repeat(64 * 1024));
  expect(waiting().length).toBe(16);
  expect(waiting().every((p) => p.text.length <= 40)).toBe(true);
  await Bun.sleep(700);
  expect(waiting().length).toBe(0);

  host.prompt("before the reload");
  host.start();
  expect(waiting().length).toBe(0);
  await live();
  host.prompt("before the end");
  host.end();
  expect(waiting().length).toBe(0);
});
