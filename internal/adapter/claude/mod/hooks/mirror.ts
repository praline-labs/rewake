// The conversation of a session rewake's own face draws (docs/face.md), sent
// to its wrapper in memory only, from a load whose hello answer said mirror.
// A hook only pushes to the feed's outbox and returns: the turn's stream
// yields every chunk unchanged, and nothing here waits on the wrapper. The
// feed (feed.ts) bounds, queues and batches; this file maps Claude Code's
// events to the mirror's.
import type { Register, TurnStepChunk } from "claude-code";
import type { HelloAnswer, MirrorBatch, MirrorEvent, MirrorPermit } from "../protocol.gen.ts";
import { paths } from "../protocol.gen.ts";
import { bounds, fit, Outbox, pieces, settles, textBudget, utf8Size } from "./feed.ts";
import type { Host } from "./outbox.ts";
import { newId } from "./outbox.ts";
import { noting, type Permits } from "./permits.ts";

// MirrorOutbox is the feed's outbox of the protocol's events.
export type MirrorOutbox = Outbox<MirrorEvent>;

// What the hooks read of the current load.
interface MirrorLoad {
  host: Host;
  run?: HelloAnswer;
  mirror?: MirrorOutbox;
  permits?: Permits;
}

// A block being put together for its settle.
interface Block {
  id: string;
  block: "text" | "thinking";
  parts: string[];
}

// stepBudget bounds what one step's open blocks and tool inputs hold
// together, in UTF-8 bytes as the outbox counts them, not in the UTF-16
// units of a string's length.
const stepBudget = 4 * bounds.block;

// shown is a tool's result as the face draws it.
const shown = (value: unknown) =>
  typeof value === "string" ? value : (JSON.stringify(value) ?? "");

// Mirror is what the hooks of the session's own conversation tell the face;
// each does nothing until the load's hello answer says mirror.
export class Mirror {
  // mode is the permission mode the harness last reported, said the one the
  // face was sent: a mode heard before the hello's answer goes with the
  // load's first event.
  private mode?: string;
  private said?: string;

  constructor(
    private current: () => MirrorLoad | undefined,
    private loadId: string,
  ) {}

  private outbox(): MirrorOutbox | undefined {
    const load = this.current();
    if (!load?.run?.mirror) return undefined;
    const host = load.host;
    load.mirror ??= new Outbox<MirrorEvent>({
      ready: () => this.current()?.run !== undefined,
      post: async (events) => {
        const run = this.current()?.run;
        if (run === undefined) return false;
        const batch: MirrorBatch = { loadId: this.loadId, generation: run.generation, events };
        return (await host.post(paths.mirrorEvents, batch)).ok;
      },
      after: (ms, run) => host.after(ms, run),
    });
    if (this.mode !== undefined && this.mode !== this.said) {
      this.said = this.mode;
      load.mirror.push({ kind: "mode", mode: this.mode });
    }
    return load.mirror;
  }

  // admit sends a mode heard before the hello's answer, once it came.
  admit() {
    this.outbox();
  }

  // moded takes the permission mode a classic event reports.
  moded(mode: string | undefined) {
    if (!mode) return;
    this.mode = mode;
    this.outbox();
  }

  prompt(text: string, origin: string, turn?: string) {
    this.outbox()?.push({ kind: "prompt", id: newId(), turn, origin, text: fit(text, textBudget) });
  }

  // submitted mirrors the face's own prompt, which the mod's hooks never see,
  // once the engine has taken it in or dropped it (after Esc or Stop it drops
  // a plugin's prompt until the person's next input).
  submitted(text: string, drop?: string) {
    const shown = drop === undefined ? text : `${text}\n\n(not sent: ${drop})`;
    this.outbox()?.push({
      kind: "prompt",
      id: newId(),
      origin: "face",
      status: drop === undefined ? undefined : "refused",
      text: fit(shown, textBudget),
    });
  }

  turn(turn: string, state: "working" | "idle") {
    this.outbox()?.push({ kind: "turn", turn, state });
  }

  toolEnded(
    id: string,
    result: { deny?: string; isError?: true; result?: unknown; text?: string },
  ) {
    const out = this.outbox();
    if (out === undefined) return;
    const failed = result.isError ? shown(result.result) : undefined;
    let permit: MirrorPermit | undefined;
    noting(() => {
      permit = this.current()?.permits?.take(id, { refused: result.deny, failed }, this.mode);
    });
    const permitted = permit ? { permit } : {};
    if (result.deny !== undefined) {
      out.push({
        kind: "result",
        id,
        status: "refused",
        text: fit(result.deny, textBudget),
        ...permitted,
      });
    } else if (failed !== undefined) {
      out.push({
        kind: "result",
        id,
        status: "error",
        text: fit(failed, textBudget),
        ...permitted,
      });
    } else {
      const text = fit(result.text ?? shown(result.result), textBudget);
      out.push({ kind: "result", id, status: "done", text, ...permitted });
    }
  }

  // step forwards a main-loop step's stream chunk by chunk, each yielded as
  // it came before anything of it is queued, and returns its result.
  async *step<R>(
    turn: string,
    index: number,
    stream: AsyncGenerator<TurnStepChunk, R>,
  ): AsyncGenerator<TurnStepChunk, R> {
    const out = this.outbox();
    if (out === undefined) return yield* stream;
    const blocks = new Map<number, Block>();
    const tools = new Map<number, { id: string; json: string; bytes: number }>();
    let held = 0;
    const settle = () => {
      for (const b of blocks.values())
        for (const event of settles(b.id, turn, b.block, b.parts.join(""))) out.push(event);
      for (const t of tools.values())
        out.push({ kind: "input", id: t.id, text: fit(t.json, textBudget) });
      blocks.clear();
      tools.clear();
    };
    for (;;) {
      const step = await stream.next();
      if (step.done) {
        settle();
        return step.value;
      }
      const c = step.value;
      yield c;
      if (c.kind === "text" || c.kind === "thinking") {
        if (c.text === "") continue;
        let b = blocks.get(c.index);
        if (b === undefined) {
          b = { id: `${turn}.${index}.${c.index}`, block: c.kind, parts: [] };
          blocks.set(c.index, b);
        }
        // Past the step's bound a block's settle would be cut anyway: its
        // deltas still go, and the face keeps what they carried.
        const size = utf8Size(c.text);
        if (held + size <= stepBudget) {
          b.parts.push(c.text);
          held += size;
        }
        for (const piece of pieces(c.text, textBudget)) {
          out.push({ kind: "delta", id: b.id, turn, block: c.kind, text: piece });
        }
      } else if (c.kind === "tool") {
        tools.set(c.index, { id: c.id, json: "", bytes: 0 });
        out.push({ kind: "tool", id: c.id, turn, name: c.name });
      } else if (c.kind === "input") {
        // A tool's input is held whole until the step stops, so it counts
        // against the step's bound as a block does.
        const t = tools.get(c.index);
        const size = utf8Size(c.json);
        if (t && t.bytes + size <= bounds.block && held + size <= stepBudget) {
          t.json += c.json;
          t.bytes += size;
          held += size;
        }
      } else if (c.kind === "stop") {
        settle();
      }
    }
  }
}

// serveMirror hooks what no other hook of the mod takes: the prompts and
// the tool calls of the main loop.
export function serveMirror(on: Parameters<Register>[0], mirror: Mirror) {
  on("prompt.submit", async ($, e, next) => {
    mirror.prompt(e.text, e.origin.kind === "composer" ? "person" : e.origin.kind, e.turnId);
    return next(e);
  });
  on("tool.call", async ($, e, next) => {
    const result = await next(e);
    if (e.agentId === undefined) mirror.toolEnded(e.tool_use_id, result);
    return result;
  });
}
