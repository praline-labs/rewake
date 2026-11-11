// A face that fails to load, throws or hangs costs its own drawing alone: the
// shell's thread hears it, keeps the last lines and goes on (docs/face.md,
// "Hot reload").
import { afterEach, expect, test } from "bun:test";
import { join } from "node:path";
import { cleanup, ends, faceFile, hosted, mirror, scratch, worker } from "./hostkit.ts";

afterEach(cleanup);

test("a face draws the wrapper's snapshot, and a reload that fails to import keeps its lines with the error", async () => {
  const dir = scratch();
  const url = faceFile(dir, `import ${JSON.stringify(worker)};\n`);
  const { host, until } = hosted(url, mirror(dir, { prompt: "say hi" }).path);
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
  host.reload();
  const failed = await until((s) => s.link === "stopped");
  expect(failed.error).toContain("missing");
  expect(failed.lines).toEqual(drawn.lines);
  expect(host.running).toBe(false);

  // Fixed again, the next reload draws afresh from the snapshot.
  faceFile(dir, `import ${JSON.stringify(worker)};\n`);
  host.reload();
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
    { op: "hello", v: 3, inputs: ["prompt"] },
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

// The face's worker selects and picks parts (face/src/face.test.ts), and the
// shell's thread copies: what crosses between them is the text alone, sent to
// the clipboard once, with the terminal program's notice.
test("a drag over the face copies the selection's text and a click the part's source", async () => {
  const dir = scratch();
  const url = faceFile(dir, `import ${JSON.stringify(worker)};\n`);
  const copies: string[] = [];
  const wrapper = mirror(dir, { replies: ["first **reply**", "second"] });
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
  host.mouse("up", 9, first, true);
  await until((s) => s.copied !== undefined);
  expect(copies).toEqual(["first re"]);
  expect(host.state.copied?.text).toBe(
    "sent 8 chars via OSC 52 · check terminal clipboard settings if paste fails",
  );

  const second = rowOf("● second");
  host.mouse("down", 4, second, true);
  host.mouse("up", 4, second, true);
  await until(() => copies.length === 2);
  expect(copies[1]).toBe("second");
  expect(host.state.copied?.text).toContain("sent the reply (6 chars) via OSC 52");
});
