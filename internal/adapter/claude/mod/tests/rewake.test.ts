import { expect, test } from "claude-code/testing";
import { utf8Length } from "../hooks/outbox.ts";
import { bounds } from "../protocol.gen.ts";
import { admitted, complete, epoch, own, socket, start, world } from "./world.ts";

test("a load says hello to its run over the socket the run names, with the next number of its counter", async ($, on) => {
  const run = world(on, { store: { ["rewake/load/" + epoch]: 4 } });
  await start($);
  expect(run.hellos().length).toBe(1);
  const hello = run.sent[0];
  expect(hello.path).toBe("/v1/hello");
  expect(hello.socket).toBe(socket);
  expect(hello.body.sessionId).toBe("conversation-1");
  expect(hello.body.harnessVersion).toBe("2.1.296");
  expect(hello.body.loadId).toMatch(/^[a-z0-9]{24}$/);
  expect(hello.body.loadSeq).toBe(5);
  expect(run.store.get("rewake/load/" + epoch)).toBe(5);
  expect(run.store.get("rewake/outboxes")).toEqual([epoch]);
});

test("a load outside a rewake run reports to nobody", async ($, on) => {
  const run = world(on, { env: {} });
  await start($);
  await complete($, "t1");
  expect(run.sent.length).toBe(0);
});

test("a refused hello leaves the session going and sends no end", async ($, on) => {
  const run = world(on, {
    replies: { "/v1/hello": [{ status: 403, text: '{"error":"process 7 is not the harness"}' }] },
  });
  const started = await start($);
  expect(started).toMatchObject({ cwd: "/work" });
  await complete($, "t1");
  await run.clock.advance(60_000);
  expect(run.sent.map((one) => one.path)).toEqual(["/v1/hello"]);
});

test("a hello that failed is sent again under the same load id and number, and its ends then go", async ($, on) => {
  const run = world(on, {
    replies: { "/v1/hello": ["throw", { status: 503, text: '{"error":"closing"}' }, admitted] },
  });
  await start($);
  await complete($, "t1");
  expect(run.ends().length).toBe(0);
  await run.clock.advance(1000);
  expect(run.sent.filter((one) => one.path === "/v1/hello").length).toBe(2);
  await run.clock.advance(2000);
  const hellos = run.sent.filter((one) => one.path === "/v1/hello");
  expect(hellos.length).toBe(3);
  expect(new Set(hellos.map((one) => one.raw)).size).toBe(1);
  await run.clock.settle();
  expect(run.ends().map((one) => one.body.turnId)).toEqual(["t1"]);
});

test("a turn start is sent once, and not before the hello is admitted", async ($, on) => {
  const run = world(on, { replies: { "/v1/turn/start": ["throw"] } });
  await $.turn.start({ text: "early", turnId: "t0" } as any);
  await start($);
  await $.turn.start({ text: "go", turnId: "t1" } as any);
  await run.clock.advance(60_000);
  const starts = run.sent.filter((one) => one.path === "/v1/turn/start");
  expect(starts.length).toBe(1);
  expect(starts[0].body).toMatchObject({ conversation: "conversation-1", turnId: "t1" });
});

test("a subagent's turn sends neither its start nor its end, an empty agent id included, while the main loop's pair goes", async ($, on) => {
  const run = world(on);
  await start($);
  await run.clock.settle();
  for (const agentId of ["agent-1", ""]) {
    await $.turn.start({ text: "read the file", turnId: "child-" + agentId, agentId } as any);
    await complete($, "child-" + agentId, { agentId });
  }
  await $.turn.start({ text: "go", turnId: "t1" } as any);
  await complete($, "t1");
  await run.clock.settle();
  expect(
    run.sent.filter((one) => one.path === "/v1/turn/start").map((one) => one.body.turnId),
  ).toEqual(["t1"]);
  expect(run.ends().map((one) => one.body.turnId)).toEqual(["t1"]);
});

test("each reason reaches the run as its end reason, and a subagent's turn is not sent", async ($, on) => {
  const run = world(on);
  await start($);
  await complete($, "t1", { reason: "answer" });
  await complete($, "t2", { reason: "aborted", isAborted: true });
  await complete($, "t3", {
    reason: "refusal",
    answer: "",
    refusal: { category: "cyber", explanation: "declined" },
  });
  await complete($, "t4", { reason: "error", answer: "" });
  await complete($, "t5", { agentId: "agent-1" });
  await run.clock.settle();
  expect(run.ends().map((one) => [one.body.turnId, one.body.reason, one.body.answer])).toEqual([
    ["t1", "answered", "done t1"],
    ["t2", "aborted", "done t2"],
    ["t3", "refused", "declined"],
    ["t4", "failed", undefined],
  ]);
  expect(run.store.has(own)).toBe(false);
});

test("the first attempt is awaited by the hook, and the end leaves the store on its receipt", async ($, on) => {
  const run = world(on);
  await start($);
  await complete($, "t1");
  expect(run.ends().length).toBe(1);
  expect(run.ends()[0].body.endId).toMatch(/^[a-z0-9]{8,}$/);
  expect(run.store.has(own)).toBe(false);
});

test("an end that comes while an older one is in flight queues behind it", async ($, on) => {
  let answered = 0;
  const run = world(on, {
    replies: {
      "/v1/turn/end": [
        async () => {
          await run.clock.sleep(5000);
          answered++;
          return "receipt";
        },
        "receipt",
      ],
    },
  });
  await start($);
  const first = complete($, "t1");
  await run.clock.settle();
  await complete($, "t2");
  expect(run.ends().map((one) => one.body.turnId)).toEqual(["t1"]);
  expect((run.store.get(own) as any[]).map((end) => end.turnId)).toEqual(["t1", "t2"]);
  await run.clock.advance(5000);
  await first;
  await run.clock.settle();
  expect(answered).toBe(1);
  expect(run.ends().map((one) => one.body.turnId)).toEqual(["t1", "t2"]);
  expect(run.store.has(own)).toBe(false);
});

test("a 503 and a 200 that is not the end's receipt are sent again, the same body each time", async ($, on) => {
  const run = world(on, {
    replies: {
      "/v1/turn/end": [
        { status: 503, text: '{"error":"unreadable"}' },
        { status: 200, text: '{"turnId":"t1","endId":"other"}' },
        "receipt",
      ],
    },
  });
  await start($);
  await complete($, "t1");
  expect(run.ends().length).toBe(1);
  await run.clock.advance(999);
  expect(run.ends().length).toBe(1);
  await run.clock.advance(1);
  expect(run.ends().length).toBe(2);
  await run.clock.advance(2000);
  expect(run.ends().length).toBe(3);
  expect(new Set(run.ends().map((one) => one.raw)).size).toBe(1);
  await run.clock.advance(60_000);
  expect(run.ends().length).toBe(3);
  expect(run.store.has(own)).toBe(false);
});

test("a 409 and a 410 are dropped and said once, and the next end goes on", async ($, on) => {
  const run = world(on, {
    replies: {
      "/v1/turn/end": [
        { status: 409, text: '{"error":"another end under this id"}' },
        { status: 410, text: '{"error":"the run is gone"}' },
        "receipt",
      ],
    },
  });
  await start($);
  await complete($, "t1");
  await complete($, "t2");
  await complete($, "t3");
  await run.clock.advance(60_000);
  expect(run.ends().map((one) => one.body.turnId)).toEqual(["t1", "t2", "t3"]);
  expect(run.logs.length).toBe(2);
  expect(run.logs[0]).toContain("409 another end under this id");
  expect(run.logs[1]).toContain("410 the run is gone");
  expect(run.store.has(own)).toBe(false);
});

for (const [name, unit] of [
  ["plain text", "a"],
  ["quotes and backslashes", '"\\'],
  ["characters outside ASCII", "жé😀"],
] as const) {
  test(
    `a long answer of ${name} is cut at the bound, with its line`,
    { timeoutMs: 30_000 },
    async ($, on) => {
      const run = world(on);
      await start($);
      const answer = unit.repeat(Math.ceil((bounds.maxEndBodyBytes * 1.2) / utf8Length(unit)));
      await complete($, "t1", { answer });
      const end = run.ends()[0];
      expect(utf8Length(end.raw) <= bounds.maxEndBodyBytes).toBe(true);
      const marked = /\n\[rewake: (\d+) characters of this answer were left out[^\]]*\]$/.exec(
        end.body.answer,
      );
      expect(marked).not.toBeNull();
      const kept = Array.from(end.body.answer.slice(0, marked!.index) as string);
      const all = Array.from(answer);
      expect(kept.join("")).toBe(all.slice(0, kept.length).join(""));
      expect(Number(marked![1])).toBe(all.length - kept.length);
      // One more character would carry the body past the bound.
      const longer = {
        ...end.body,
        answer:
          all.slice(0, kept.length + 1).join("") +
          `\n[rewake: ${all.length - kept.length - 1}` +
          marked![0].slice(marked![0].indexOf(" characters")),
      };
      expect(utf8Length(JSON.stringify(longer)) > bounds.maxEndBodyBytes).toBe(true);
    },
  );
}

test("the store's ends of the run go first after a new load's hello, under their own end ids and the new load's id; another run's stay", async ($, on) => {
  const other = "9.200.boot";
  const kept = {
    loadId: "old",
    conversation: "conversation-1",
    turnId: "t0",
    endId: "end-old",
    reason: "answered",
    answer: "before the reload",
  };
  const theirs = {
    loadId: "theirs",
    conversation: "conversation-9",
    turnId: "x",
    endId: "end-theirs",
    reason: "answered",
  };
  const run = world(on, {
    store: {
      [own]: [kept],
      ["rewake/outbox/" + other]: [theirs],
      "rewake/outboxes": [other, epoch],
    },
  });
  await start($);
  await run.clock.settle();
  await complete($, "t1");
  await run.clock.settle();
  expect(run.ends().map((one) => one.body.endId)).toEqual(["end-old", run.ends()[1].body.endId]);
  expect(run.ends()[0].body).toEqual({ ...kept, loadId: run.sent[0].body.loadId });
  expect(run.ends()[1].body.turnId).toBe("t1");
  expect(run.sent.find((one) => one.path === "/v1/mod/ended")!.body.epochs).toEqual([other]);
  expect(run.store.get("rewake/outbox/" + other)).toEqual([theirs]);
  expect(run.store.has(own)).toBe(false);
});

test("only the keys of runs mod/ended answers ended are removed", async ($, on) => {
  const [gone, alive] = ["1.1.boot", "2.2.boot"];
  const run = world(on, {
    replies: {
      "/v1/mod/ended": [{ status: 200, text: JSON.stringify({ ended: [gone, "never-asked"] }) }],
    },
    store: {
      ["rewake/outbox/" + gone]: [{ endId: "g" }],
      ["rewake/load/" + gone]: 3,
      ["rewake/outbox/" + alive]: [{ endId: "a" }],
      ["rewake/load/" + alive]: 1,
      "rewake/outboxes": [gone, alive],
    },
  });
  await start($);
  expect(run.store.has("rewake/outbox/" + gone)).toBe(false);
  expect(run.store.has("rewake/load/" + gone)).toBe(false);
  expect(run.store.get("rewake/outbox/" + alive)).toEqual([{ endId: "a" }]);
  expect(run.store.get("rewake/load/" + alive)).toBe(1);
  expect(run.store.get("rewake/outboxes")).toEqual([alive, epoch]);
});

test("a cleanup cut short loses no end of the run's own", async ($, on) => {
  const gone = "1.1.boot";
  const kept = {
    loadId: "old",
    conversation: "conversation-1",
    turnId: "t0",
    endId: "end-old",
    reason: "answered",
  };
  const run = world(on, {
    replies: { "/v1/mod/ended": [{ status: 200, text: JSON.stringify({ ended: [gone] }) }] },
    store: {
      [own]: [kept],
      ["rewake/outbox/" + gone]: [{ endId: "g" }],
      "rewake/outboxes": [gone, epoch],
    },
    refused: "rewake/load/" + gone,
  });
  await start($);
  await run.clock.settle();
  expect(run.ends().map((one) => one.body.endId)).toEqual(["end-old"]);
  expect(run.store.has(own)).toBe(false);
  expect(run.store.get("rewake/outboxes")).toEqual([gone, epoch]);
});

test("review: a new end before a retried hello preserves the older durable end", async ($, on) => {
  const kept = {
    loadId: "old",
    conversation: "conversation-1",
    turnId: "t0",
    endId: "end-old",
    reason: "answered",
    answer: "old report",
  };
  const run = world(on, {
    replies: { "/v1/hello": ["throw", admitted] },
    store: { [own]: [kept] },
  });
  await start($);
  await complete($, "t1");
  await run.clock.advance(1000);
  await run.clock.settle();
  expect(run.ends().map((one) => one.body.turnId)).toEqual(["t0", "t1"]);
});

test("review: a restored end from a load whose hello never arrived remains retryable", async ($, on) => {
  const kept = {
    loadId: "never-admitted",
    conversation: "conversation-1",
    turnId: "t0",
    endId: "end-old",
    reason: "answered",
    answer: "old report",
  };
  const run = world(on, {
    replies: {
      "/v1/turn/end": [
        async (body) =>
          body.loadId === "never-admitted"
            ? {
                status: 400,
                text: '{"error":"no hello of this load was admitted in this run, so its turns are not taken"}',
              }
            : "receipt",
      ],
    },
    store: { [own]: [kept] },
  });
  await start($);
  await run.clock.settle();
  const delivered = run
    .ends()
    .some((one) => one.body.endId === "end-old" && one.body.loadId !== "never-admitted");
  const retained =
    (run.store.get(own) as any[] | undefined)?.some((one) => one.endId === "end-old") ?? false;
  expect(delivered || retained).toBe(true);
});

test("a read of the store that fails leaves the stored ends in place, and they go first once it reads", async ($, on) => {
  const kept = {
    loadId: "old",
    conversation: "conversation-1",
    turnId: "t0",
    endId: "end-old",
    reason: "answered",
    answer: "old report",
  };
  const run = world(on, { store: { [own]: [kept] }, unreadable: 2 });
  await start($);
  await complete($, "t1");
  // A reload now would find the store as the earlier load left it.
  expect(run.store.get(own)).toEqual([kept]);
  expect(run.ends().length).toBe(0);
  expect(run.logs.filter((line) => line.includes("could not be read")).length).toBe(1);
  await run.clock.advance(1000);
  await run.clock.advance(2000);
  await run.clock.settle();
  expect(run.ends().map((one) => one.body.turnId)).toEqual(["t0", "t1"]);
  expect(run.store.has(own)).toBe(false);
});

test("every id the mod makes has one length, so an end sent under a later load keeps its measured size", async ($, on) => {
  const run = world(on);
  await start($);
  await complete($, "t1");
  expect(run.ends()[0].body.endId).toMatch(/^[a-z0-9]{24}$/);
  expect(run.ends()[0].body.loadId).toMatch(/^[a-z0-9]{24}$/);
});
