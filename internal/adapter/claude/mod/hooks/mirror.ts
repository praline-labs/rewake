// The conversation of a session rewake's own face draws (docs/face.md), sent
// to its wrapper in memory only, from a load whose hello answer said mirror.
// A hook only appends to the outbox and returns: the turn's stream yields
// every chunk unchanged, and nothing here waits on the wrapper. The outbox is
// bounded; past its bound the oldest deltas go first, which the block's
// settle heals, then the oldest other events, each leaving a lost marker,
// and when the markers fill their reserve they collapse into one resync.
import type { Register, TurnStepChunk } from "claude-code";
import type { HelloAnswer, MirrorBatch, MirrorEvent, MirrorPermit } from "../protocol.gen.ts";
import { bounds, paths } from "../protocol.gen.ts";
import type { Host } from "./outbox.ts";
import { newId } from "./outbox.ts";
import { noting, type Permits } from "./permits.ts";
import { fit, jsonSize, pieces, utf8Size } from "./sizes.ts";

// Room for an event's own fields: a text of an event stays within this.
const textBudget = bounds.maxMirrorEventBytes - 4096;
const markerSpace = bounds.mirrorMarkerBytes;
const eventSpace = bounds.maxMirrorOutboxBytes - markerSpace;
const batchBudget = bounds.maxMirrorBatchBytes - 256;

interface Queued {
  event: MirrorEvent;
  size: number;
  marker: boolean;
}

const isMarker = (event: MirrorEvent) =>
  event.kind === "turn" || event.kind === "lost" || event.kind === "resync";

// MirrorOutbox holds the events not sent yet, within its bounds, and sends
// them in batches, one in flight.
export class MirrorOutbox {
  private queue: Queued[] = [];
  private events = 0;
  private markers = 0;
  // lost counts what was dropped since the last resync was queued.
  private lost = 0;
  private sending = false;
  private scheduled = false;

  constructor(
    private host: Host,
    private load: () => { loadId: string; generation: number } | undefined,
  ) {}

  // size is what the outbox holds, events and markers.
  get bytes() {
    return this.events + this.markers;
  }

  push(event: MirrorEvent) {
    const size = utf8Size(JSON.stringify(event));
    if (isMarker(event)) {
      while (this.markers + size > markerSpace && this.collapse()) {}
      this.markers += size;
    } else {
      while (this.events + size > eventSpace && this.dropOne()) {}
      this.events += size;
    }
    this.queue.push({ event, size, marker: isMarker(event) });
    this.schedule();
  }

  // dropOne drops the oldest delta, or else the oldest other event with
  // every part of its block, leaving a lost marker for it.
  private dropOne(): boolean {
    let at = this.queue.findIndex((q) => q.event.kind === "delta");
    if (at >= 0) {
      this.events -= this.queue[at].size;
      this.queue.splice(at, 1);
      this.lost++;
      return true;
    }
    at = this.queue.findIndex((q) => !q.marker);
    if (at < 0) return false;
    const id = this.queue[at].event.id;
    const kind = this.queue[at].event.kind;
    this.queue = this.queue.filter((q) => {
      const same = !q.marker && q.event.id === id && (q.event.kind === kind || kind !== "settle");
      if (same) {
        this.events -= q.size;
        this.lost++;
      }
      return !same;
    });
    if (id !== undefined) this.push({ kind: "lost", id });
    return true;
  }

  // collapse turns every queued marker into one resync standing for them.
  private collapse(): boolean {
    if (this.markers === 0) return false;
    let collapsed = 0;
    this.queue = this.queue.filter((q) => {
      if (q.marker) {
        collapsed += q.event.kind === "resync" ? (q.event.lost ?? 0) : 1;
        this.markers -= q.size;
      }
      return !q.marker;
    });
    this.lost += collapsed;
    const resync: MirrorEvent = { kind: "resync", lost: this.lost };
    this.lost = 0;
    const size = utf8Size(JSON.stringify(resync));
    this.markers += size;
    this.queue.unshift({ event: resync, size, marker: true });
    return true;
  }

  private schedule() {
    if (this.scheduled || this.sending) return;
    this.scheduled = true;
    this.host.after(50, () => {
      this.scheduled = false;
      void this.send();
    });
  }

  // send posts the oldest events, at most a batch; a batch that fails or
  // does not answer within 5 s is dropped, never sent again, and stands as
  // lost behind a resync.
  async send() {
    const load = this.load();
    if (this.sending || this.queue.length === 0 || load === undefined) return;
    const events: MirrorEvent[] = [];
    let size = 0;
    while (
      this.queue.length > 0 &&
      (events.length === 0 || size + this.queue[0].size <= batchBudget)
    ) {
      const q = this.queue.shift()!;
      if (q.marker) this.markers -= q.size;
      else this.events -= q.size;
      size += q.size;
      events.push(q.event);
    }
    this.sending = true;
    const batch: MirrorBatch = { loadId: load.loadId, generation: load.generation, events };
    let done = false;
    const failed = () => {
      if (done) return;
      done = true;
      this.sending = false;
      this.lost += events.length;
      const lost = this.lost;
      this.lost = 0;
      this.push({ kind: "resync", lost });
    };
    this.host.after(5000, failed);
    try {
      const answer = await this.host.post(paths.mirrorEvents, batch);
      if (!answer.ok) {
        failed();
        return;
      }
    } catch {
      failed();
      return;
    }
    if (done) return;
    done = true;
    this.sending = false;
    if (this.queue.length > 0) this.schedule();
  }
}

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

// settles is a block's settle in parts, each within an event.
export function settles(turn: string, b: Block): MirrorEvent[] {
  const text = fit(b.parts.join(""), bounds.maxMirrorBlockBytes);
  const parts = pieces(text, textBudget);
  return parts.map((part, i) => ({
    kind: "settle",
    id: b.id,
    turn,
    block: b.block,
    part: i,
    of: parts.length,
    text: part,
  }));
}

// stepBudget bounds what one step's open blocks and tool inputs hold
// together, in UTF-8 bytes as the outbox counts them, not in the UTF-16
// units of a string's length.
const stepBudget = 4 * bounds.maxMirrorBlockBytes;

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
    load.mirror ??= new MirrorOutbox(load.host, () => {
      const run = this.current()?.run;
      return run ? { loadId: this.loadId, generation: run.generation } : undefined;
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
      for (const b of blocks.values()) for (const event of settles(turn, b)) out.push(event);
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
        if (t && t.bytes + size <= bounds.maxMirrorBlockBytes && held + size <= stepBudget) {
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
