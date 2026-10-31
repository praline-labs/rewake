// A face that breaks the contract with its shell, or speaks another one, is
// stopped or draws nothing, and what it sends reaches the shell only as the
// shell's own copies (docs/face.md, "Hot reload").
import { afterEach, expect, test } from "bun:test";
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
