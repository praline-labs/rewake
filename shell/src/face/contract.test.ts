// A face that breaks the contract with its shell, or speaks another one, is
// stopped or draws nothing, and what it sends reaches the shell only as the
// shell's own copies (docs/face.md, "Hot reload").
import { afterEach, expect, test } from "bun:test";
import { contract } from "../../../face/src/messages.ts";
import type { FaceState } from "./host.ts";
import { cleanup, ends, faceFile, hosted, mirror, scratch, worker } from "./hostkit.ts";

// Solid's client build, whose Index takes a copy of a list on its update as
// the shell's renderer does; a test resolves solid-js to its server build.
const client = "solid-js/dist/solid.js";
const { createRoot, createSignal, indexArray }: typeof import("solid-js") = await import(client);

afterEach(cleanup);

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

// Review of 288cd7f: a face refused for its contract stays refused; what the
// shell sends after start, its ctrl+o among it, leaves the last good frame
// and the error. A start, unlike a reload, has no running face to keep.
test("a face started again in another contract says why, and the last good frame stays", async () => {
  const dir = scratch();
  const url = faceFile(dir, `import ${JSON.stringify(worker)};\n`);
  const { host, until } = hosted(url, mirror(dir, { prompt: "say hi" }).path);
  host.start();
  const good = await until((s) => s.link === "live" && s.lines.length > 0);
  host.unfold();
  // The real face, told by its shell of a contract one past its own.
  faceFile(
    dir,
    `import ${JSON.stringify(worker)};
const real = self.onmessage;
self.onmessage = (e) => real({ data: e.data.kind === "start" ? { ...e.data, contract: e.data.contract + 1 } : e.data });\n`,
  );
  host.start();
  const said = await until((s) => s.error !== undefined);
  expect(said.error).toMatch(/^this face speaks contract \d+, the shell \d+$/);
  host.size(41, 30);
  host.scroll(1);
  await Bun.sleep(200);
  expect(host.state.lines).toEqual(good.lines);
  expect(host.running).toBe(true);
});

test("rows and spans a face sends reach the shell only as its own copies", async () => {
  const dir = scratch();
  // A structured clone keeps an array's own properties: a row list and a
  // span list each carrying a `slice` of their own, which Solid's Index calls.
  const url = faceFile(
    dir,
    `self.onmessage = (e) => {
  if (e.data.kind !== "start") return;
  const spans = Object.assign([{ text: "x", extra: 1 }], { slice: 0 });
  const lines = Object.assign([{ spans, extra: 2 }], { slice: 0 });
  self.postMessage({ kind: "lines", lines, turn: "idle", link: "live", prompts: false, extra: 3 });
};\n`,
  );
  const { host, until } = hosted(url);
  host.start();
  const drawn = await until((s) => s.link === "live");
  // Drawn as FaceView draws rows and a row's spans, with Index, which takes a
  // copy of a list on its update.
  createRoot((dispose) => {
    ends.push(dispose);
    for (const list of [drawn.lines, drawn.lines[0]?.spans ?? []]) {
      const [items, setItems] = createSignal<unknown[]>([]);
      const mapped = indexArray(items, () => null);
      mapped();
      setItems(list);
      mapped();
    }
  });
  expect(drawn.lines).toEqual([{ spans: [{ text: "x" }] }]);
  expect(Object.keys(drawn.lines)).toEqual(["0"]);
  expect(Object.keys(drawn.lines[0]?.spans ?? {})).toEqual(["0"]);
  expect(host.running).toBe(true);
});

const texts = (s: FaceState) =>
  s.lines.map((l) =>
    l.spans
      .map((sp) => sp.text)
      .join("")
      .trimEnd(),
  );
const shows = (text: string) => (s: FaceState) => s.link === "live" && texts(s).includes(text);

// November 1, 2026: a shell of contract 4 reloaded faces of contract 5 from
// the checkout, and the pane froze on the refusal until a restart.
test("a reload into a face of another contract keeps the running face drawing, said once", async () => {
  const dir = scratch();
  const url = faceFile(dir, `import ${JSON.stringify(worker)};\n`);
  const wrapper = mirror(dir, { prompt: "say hi" });
  const { host, states, until } = hosted(url, wrapper.path, { pingEvery: 50, noticeFor: 300 });
  host.start();
  await until(shows("❯ say hi"));
  // A face of the next contract, refusing this shell as a real one does.
  const messages = new URL("../../../face/src/messages.ts", import.meta.url).pathname;
  faceFile(
    dir,
    `import { refusal } from ${JSON.stringify(messages)};
self.onmessage = (e) => {
  if (e.data.kind === "start") self.postMessage({ kind: "error", text: refusal(e.data.contract + 1, e.data.contract) });
  if (e.data.kind === "ping") self.postMessage({ kind: "pong", n: e.data.n });
};\n`,
  );
  host.reload();
  const told = await until((s) => s.notice !== undefined);
  expect(told.notice).toBe(
    `the face on disk speaks contract ${contract + 1}, this shell ${contract}: a restart of the room loads it`,
  );
  expect(told.error).toBeUndefined();
  // The running face still hears the mirror and draws.
  wrapper.send({
    op: "add",
    item: { id: "r", kind: "text", status: "settled", text: "still here" },
  });
  await until(shows("● still here"));
  await until((s) => s.notice === undefined);
  // The same contract met again is not said again; the face is not stopped.
  host.reload();
  await Bun.sleep(200);
  expect(states.filter((s) => s.notice !== undefined).length).toBeGreaterThan(0);
  expect(host.state.notice).toBeUndefined();
  expect(host.state.link).toBe("live");
  expect(host.running).toBe(true);
});

test("a reload of the same contract swaps the faces", async () => {
  const dir = scratch();
  const url = faceFile(dir, `import ${JSON.stringify(worker)};\n`);
  const { host, until } = hosted(url, mirror(dir, { prompt: "say hi" }).path, { pingEvery: 50 });
  host.start();
  await until(shows("❯ say hi"));
  faceFile(
    dir,
    `self.onmessage = (e) => {
  if (e.data.kind === "start")
    self.postMessage({ kind: "lines", lines: [{ spans: [{ text: "new face" }] }], turn: "idle", link: "live", prompts: false });
  if (e.data.kind === "ping") self.postMessage({ kind: "pong", n: e.data.n });
};\n`,
  );
  host.reload();
  await until(shows("new face"));
  // Its pings are answered, so it is not stopped as hung.
  await Bun.sleep(200);
  expect(host.state.link).toBe("live");
  expect(host.state.notice).toBeUndefined();
});
