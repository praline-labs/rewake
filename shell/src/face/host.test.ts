// A face that fails to load, throws or hangs costs its own drawing alone: the
// shell's thread hears it, keeps the last lines and goes on (docs/face.md,
// "Hot reload").
import { afterEach, expect, test } from "bun:test";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { FaceHost, type FaceState } from "./host.ts";

const worker = new URL("../../../face/src/worker.ts", import.meta.url).pathname;
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

// A wrapper's mirror socket serving one snapshot of one prompt, or of the
// replies given; send writes a change to the face connected last.
function mirror(dir: string, text: string, replies: string[] = []) {
  const path = join(dir, "m.sock");
  const items = replies.length
    ? replies.map((text, i) => ({ id: `r${i}`, kind: "text", status: "settled", text }))
    : [{ id: "p", kind: "prompt", status: "settled", text }];
  const lines = [
    { op: "hello", v: 1, inputs: ["prompt"] },
    { op: "begin", items: items.length, dropped: 0, turn: "idle" },
    ...items.map((item) => ({ op: "item", item })),
    { op: "end" },
  ];
  let last: { write(data: string): number } | undefined;
  const server = Bun.listen({
    unix: path,
    socket: {
      open(s) {
        last = s;
        s.write(lines.map((l) => `${JSON.stringify(l)}\n`).join(""));
      },
      data() {},
    },
  });
  ends.push(() => server.stop(true));
  const send = (line: object) => last?.write(`${JSON.stringify(line)}\n`);
  return { path, send };
}

test("a face draws the wrapper's snapshot, and a reload that fails to import keeps its lines with the error", async () => {
  const dir = scratch();
  const url = faceFile(dir, `import ${JSON.stringify(worker)};\n`);
  const { host, until } = hosted(url, mirror(dir, "say hi").path);
  host.start();
  const drawn = await until((s) => s.link === "live" && s.lines.length > 0);
  // Every row is filled to the pane, so the layer is opaque.
  expect(drawn.lines.length).toBe(24);
  const text = drawn.lines.map((l) =>
    l.spans
      .map((s) => s.text)
      .join("")
      .trimEnd(),
  );
  expect(text.filter(Boolean)).toEqual(["❯ say hi"]);

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

const shownText = (s: FaceState) =>
  s.lines.map((l) =>
    l.spans
      .map((x) => x.text)
      .join("")
      .trim(),
  );

// Seen live: the first notch from the bottom was counted against the tail
// the face had laid out, not the whole conversation, and jumped rows.
test("a notch of the wheel moves the face one row, from the bottom and on", async () => {
  const dir = scratch();
  const replies = Array.from({ length: 50 }, (_, i) => `line ${i + 1}`);
  const { host, until } = hosted(worker, mirror(dir, "", replies).path);
  host.start();
  const at = (line: string) => until((s) => shownText(s).at(0) === line);
  // Each reply is a row and a blank one: the last 24 rows start at a blank.
  await until((s) => s.link === "live" && shownText(s).at(-1) === "● line 50");
  expect(shownText(host.state).slice(0, 2)).toEqual(["", "● line 39"]);
  host.wheel(true);
  await at("● line 38");
  host.wheel(true);
  await at("");
  // Down: the first notch back is held, as natively, the next one moves.
  host.wheel(false);
  host.wheel(false);
  await at("● line 38");
  host.scroll(-100);
  await until((s) => shownText(s).at(1) === "● line 39");
});

// Seen in review: a change that came after a tick was laid out and painted
// by a notch before the next tick, ahead of the face's one clock.
test("a notch cuts the rows the last tick painted; a change after it waits for its tick", async () => {
  const dir = scratch();
  const replies = Array.from({ length: 50 }, (_, i) => `line ${i + 1}`);
  const wrapper = mirror(dir, "", replies);
  const { host, states, until } = hosted(worker, wrapper.path);
  host.start();
  await until((s) => s.link === "live" && shownText(s).at(-1) === "● line 50");
  // The clock stands once the snapshot is painted: the change starts it, and
  // its tick comes 50 ms later; the notch comes well before.
  await Bun.sleep(150);
  wrapper.send({ op: "append", id: "r48", text: " NEW" });
  await Bun.sleep(15);
  const before = states.length;
  host.wheel(true);
  const scrolled = await until((s) => shownText(s).at(0) === "● line 38");
  expect(shownText(scrolled)).toContain("● line 49");
  expect(states.slice(before, states.indexOf(scrolled) + 1).flatMap(shownText)).not.toContain(
    "● line 49 NEW",
  );
  await until((s) => shownText(s).includes("● line 49 NEW"));
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

// The face's worker selects and picks parts, and the shell's thread copies:
// what crosses between them is the text alone, sent to the clipboard once.
test("a drag over the face copies the selection's text and a click the part's source", async () => {
  const dir = scratch();
  const url = faceFile(dir, `import ${JSON.stringify(worker)};\n`);
  const copies: string[] = [];
  const wrapper = mirror(dir, "", ["first **reply**", "second"]);
  const { host, until } = hosted(url, wrapper.path, {
    copy: (text: string) => copies.push(text) > 0,
  });
  host.start();
  const drawn = await until((s) => s.lines.some((l) => l.spans.some((sp) => sp.text === "second")));
  const rowOf = (text: string) =>
    drawn.lines.findIndex((l) =>
      l.spans
        .map((s) => s.text)
        .join("")
        .startsWith(text),
    );
  const first = rowOf("● first");

  host.mouse("down", 2, first, true);
  host.mouse("drag", 9, first, true);
  const selected = await until((s) => s.lines[first]?.spans.some((sp) => sp.selected) ?? false);
  expect(selected.lines[first]?.spans.filter((s) => s.selected).map((s) => s.text)).toEqual([
    "first ",
    "re",
  ]);
  host.mouse("up", 9, first, true);
  await until((s) => s.copied !== undefined);
  expect(copies).toEqual(["first re"]);
  expect(host.state.copied?.text).toBe(
    "sent 8 chars via OSC 52 · check terminal clipboard settings if paste fails",
  );

  // A click on the other reply copies it as written, and drops the selection.
  const second = rowOf("● second");
  host.mouse("down", 4, second, true);
  host.mouse("up", 4, second, true);
  await until(() => copies.length === 2);
  expect(copies[1]).toBe("second");
  await until((s) => !s.lines[first]?.spans.some((sp) => sp.selected));
  expect(host.state.copied?.text).toContain("sent the reply (6 chars) via OSC 52");
  // The blank above the short conversation is no part: nothing copied by a
  // click there.
  expect(first).toBeGreaterThan(0);
  host.mouse("down", 4, 0, true);
  host.mouse("up", 4, 0, true);
  host.mouse("down", 4, second, true);
  host.mouse("up", 4, second, true);
  await until(() => copies.at(-1) === "second" && copies.length > 2);
  expect(copies.slice(2)).toEqual(["second"]);
  // Nothing lights as the pointer moves (the owner's word, October 31, 2026),
  // even for a move an older shell still passes; a click lights the part it
  // copied for a moment, with the notice, and then it goes out.
  await until((s) => !s.lines.some((l) => l.lit));
  host.mouse("move" as never, 4, first, true);
  await Bun.sleep(100);
  expect(host.state.lines.some((l) => l.lit)).toBe(false);
  host.mouse("down", 4, first, true);
  host.mouse("up", 4, first, true);
  await until((s) => s.lines[first]?.lit === true);
  expect(copies.at(-1)).toBe("first **reply**");
  expect(host.state.lines[second]?.lit).toBeUndefined();
  await until((s) => !s.lines[first]?.lit);
  expect(host.state.copied).toBeDefined();
});

test("a face that posts what the contract does not allow is stopped, and nothing of it is kept", async () => {
  const dir = scratch();
  const url = faceFile(
    dir,
    `self.onmessage = (e) => {
  if (e.data.kind === "start")
    self.postMessage({ kind: "lines", lines: [{ spans: [{ text: 5 }] }], turn: "idle", link: "live", prompts: false });
};\n`,
  );
  const { host, until } = hosted(url);
  host.start();
  const stopped = await until((s) => s.link === "stopped");
  expect(stopped.error).toBe("the face sent what the contract does not allow: a span's text");
  expect(stopped.lines).toEqual([]);
  expect(host.running).toBe(false);
});

test("a face of another contract draws nothing and says why", async () => {
  const dir = scratch();
  // The real face, told by its shell of a contract one past its own.
  const url = faceFile(
    dir,
    `import ${JSON.stringify(worker)};
const real = self.onmessage;
self.onmessage = (e) => real({ data: e.data.kind === "start" ? { ...e.data, contract: e.data.contract + 1 } : e.data });\n`,
  );
  const { host, until } = hosted(url, mirror(dir, "say hi").path);
  host.start();
  const said = await until((s) => s.error !== undefined);
  expect(said.error).toMatch(/^this face speaks contract \d+, the shell \d+$/);
  await Bun.sleep(200);
  expect(host.state.lines).toEqual([]);
  expect(host.state.link).toBe("connecting");
});
