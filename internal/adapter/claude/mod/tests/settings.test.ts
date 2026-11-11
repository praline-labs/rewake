// The run's status line in the settings Claude Code reads
// (docs/claude.md#the-cache-ttl): a launch that passes a --settings of its own,
// or a --command wrapper that adds one, leaves the last layer of the read to
// that layer, and rewake adds no competing one, so the mod puts the launch's
// statusLine into the settings the engine runs under. The test kit fires no
// settings read (the test's $ has no settings method, 2.1.322), so these run the
// module's own registration as the engine would and call its hook with the
// event and the next handler the engine gives it.
import { expect, test } from "claude-code/testing";
import { register } from "../hooks/rewake.ts";

// The statusLine setting a run's launch names for its mod, as the launch passes
// it: the JSON of what would otherwise stand in the run's --settings.
const carried = JSON.stringify({
  type: "command",
  command: "'/room/bin/rewake' statusline",
  refreshInterval: 30,
});

type Handler = (...args: any[]) => any;

// hooksOf runs the module's registration and keeps what it registered under
// each event, as the engine holds it.
function hooksOf(): Record<string, Handler[]> {
  const seen: Record<string, Handler[]> = {};
  const on = (name: string, ...rest: any[]) => {
    const handler: Handler = rest.length > 1 ? rest[1] : rest[0];
    const listed = seen[name] ?? [];
    listed.push(handler);
    seen[name] = listed;
    return { catch: () => undefined };
  };
  register(on as any);
  return seen;
}

// readThrough runs a settings read through the module's hooks as the engine
// does: each one's next calls the one beneath, and the last reaches the engine.
function readThrough(
  hooks: Record<string, Handler[]>,
  e: Record<string, unknown>,
  engine: (e: Record<string, unknown>) => Promise<any>,
  line: string | undefined,
) {
  const $ = {
    env: { get: async (name: string) => (name === "REWAKE_STATUS_LINE" ? line : undefined) },
  };
  const chain = hooks["settings.read"] ?? [];
  const step = async (index: number, event: Record<string, unknown>): Promise<any> => {
    if (index === chain.length) return engine(event);
    return chain[index]($, event, (rewritten: Record<string, unknown>) =>
      step(index + 1, rewritten),
    );
  };
  return step(0, e);
}

// wrapped is the layers a main's read is made of when the wrapper named with
// --command adds its own --settings: the person's user layer, the folder's
// project layer and the wrapper's flag layer, which is the last and holds no
// statusLine — what the live wrapper does, its crossSessionInbound and model
// mapping only.
const wrapped = [
  { source: "user", settings: { model: "person's" } },
  { source: "project", settings: { statusLine: { type: "command", command: "echo OWNER-LINE" } } },
  { source: "flag", settings: { crossSessionInbound: "accept", model: "wrapper's" } },
];

// mergedOf is what the layers come to, as next answers with them: the last
// layer that sets a key holds it, and the person's project statusLine stands.
const mergedOf = {
  model: "wrapper's",
  crossSessionInbound: "accept",
  statusLine: { type: "command", command: "echo OWNER-LINE" },
};

test("a wrapped launch's read keeps rewake's status line and the wrapper's own settings", async () => {
  const read = await readThrough(
    hooksOf(),
    { layers: wrapped },
    async () => ({ value: mergedOf }),
    carried,
  );
  expect(read.value.statusLine).toEqual(JSON.parse(carried));
  expect(read.value.crossSessionInbound).toBe("accept");
  expect(read.value.model).toBe("wrapper's");
});

test("a managed status line is the administrator's and is left alone", async () => {
  const managed = { type: "command", command: "echo MANAGED-LINE" };
  const layers = [...wrapped, { source: "policy", settings: { statusLine: managed } }];
  const read = await readThrough(
    hooksOf(),
    { layers },
    async () => ({ value: { ...mergedOf, statusLine: managed } }),
    carried,
  );
  expect(read.value.statusLine).toEqual(managed);
});

test("a read of one source, and one outside a rewake run, are passed through untouched", async () => {
  const one = { statusLine: { type: "command", command: "mine" } };
  const scoped = await readThrough(
    hooksOf(),
    { source: "project", layers: [wrapped[1]] },
    async () => ({ value: one }),
    carried,
  );
  expect(scoped.value).toEqual(one);
  const outside = await readThrough(
    hooksOf(),
    { layers: wrapped },
    async () => ({ value: mergedOf }),
    undefined,
  );
  expect(outside.value).toEqual(mergedOf);
});

test("a read a hook beneath refused is given back as it came", async () => {
  const read = await readThrough(
    hooksOf(),
    { layers: wrapped },
    async () => ({ deny: "the settings are locked" }),
    carried,
  );
  expect(read).toEqual({ deny: "the settings are locked" });
});

test("a carried setting that is no status line leaves the read as it is", async () => {
  const read = await readThrough(
    hooksOf(),
    { layers: wrapped },
    async () => ({ value: mergedOf }),
    "{not json",
  );
  expect(read.value).toEqual(mergedOf);
});
