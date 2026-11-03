// Native state and control stay outside the heartbeat's awaited chain: a
// compaction may take minutes while the load must keep renewing its lease.
// A command is carried out natively after the check made just before its
// call, and answered as it resolved: the host words every reply, waits for
// an interrupted turn to settle and names who stopped it (modhost/control.go).

import type {
  AskKind,
  ControlCode,
  ControlCommand,
  ControlReply,
  StateRequest,
} from "../protocol.gen.ts";
import { paths } from "../protocol.gen.ts";
import type { Host } from "./outbox.ts";
import { lastRetryMs, nextDelay } from "./outbox.ts";

export interface Native {
  id: () => Promise<string>;
  model: () => Promise<string>;
  // cwd is the directory the session runs in now: a worktree entry moves it.
  cwd: () => Promise<string>;
  // usage is $.session.usage() as Claude Code answers it; the host reads it.
  usage: () => Promise<unknown>;
  compact: (
    focus: string,
  ) => Promise<{ skip?: string; tokensBefore?: number; tokensAfter?: number }>;
  abort: (turnId: string) => Promise<void>;
  trace: (text: string) => Promise<unknown>;
  // submit sends a prompt of rewake's face as the person's (mirror.ts).
  submit: (text: string) => Promise<unknown>;
}

type Result = { outcome: ControlReply["outcome"]; code?: ControlCode; native?: unknown };

// thrown is what a native call threw, as its message.
function thrown(error: unknown): unknown {
  return { error: error instanceof Error ? error.message : String(error) };
}

export class Management {
  private generation = 0;
  private sequence = 0;
  private compactions = 0;
  private last = { request: "", from: "" };
  private turn?: string;
  // step is the last main-loop step as read: the effort its request asked
  // for and the usage its response reported, the cache's only source.
  private step: { effort?: unknown; usage?: unknown } = {};
  private compacting = false;
  // dialog is what the engine said of a dialog the session waits at that no
  // request carries to main (dialogs.ts), until the next tool result or step.
  private dialog?: string;
  // held are what the dialogs a request carries to main ask, while it holds
  // them (dialogs.ts).
  private held: AskKind[] = [];
  private remote?: ControlCommand;
  // interrupting is the turn an interrupt stopped, in flight until it ends.
  private interrupting?: string;
  private sending = Promise.resolve();
  private seen = new Set<string>();

  constructor(
    private readonly host: Host,
    private readonly native: Native,
    private readonly loadId: string,
  ) {}

  admit(generation: number) {
    this.generation = generation;
    this.changed();
  }

  started(turnId: string) {
    this.turn = turnId;
    this.changed();
  }

  // A main-loop request names the effort the session runs at now; the person
  // may change it between turns, and no other source of the engine says it.
  stepStarted(effort: unknown) {
    if (effort === this.step.effort) return;
    this.step = { ...this.step, effort };
    this.changed();
  }

  // stepEnded keeps the usage a main-loop response reported, as reported;
  // the next sample carries it.
  stepEnded(usage: unknown) {
    this.step = { ...this.step, usage };
  }

  // waits says the session waits at a dialog nobody else is told of, or with
  // no message that it no longer does.
  waits(message?: string) {
    if (message === this.dialog) return;
    this.dialog = message;
    this.changed();
  }

  // holds says the session waits at a dialog asking kind while a request
  // carries it, until the returned release.
  holds(kind: AskKind): () => void {
    this.held.push(kind);
    this.changed();
    let released = false;
    return () => {
      if (released) return;
      released = true;
      this.held.splice(this.held.indexOf(kind), 1);
      this.changed();
    };
  }

  // A turn's end frees an interrupt of it; the host matches the end it hears
  // on the outbox to the interrupt.
  ended(turnId: string) {
    if (this.turn === turnId) this.turn = undefined;
    if (this.interrupting === turnId) this.interrupting = undefined;
    this.dialog = undefined;
    this.changed();
  }

  // Samples are serialized and complete: what the session read of itself
  // goes as read, and a read that failed is absent, so an unknown usage
  // clears an older one instead of keeping pre-compaction tokens as current.
  changed() {
    if (!this.generation) return;
    const state = {
      generation: this.generation,
      activity: this.turn ? "working" : "idle",
      turn: this.turn,
      compacting: this.compacting,
      compactions: this.compactions,
      dialog: this.dialog,
      // A permission stops the call whatever else is asked beside it.
      waiting: this.held.includes("permission") ? ("permission" as const) : this.held[0],
      ...this.last,
    };
    const step = this.step;
    this.sending = this.sending
      .then(async () => {
        if (state.generation !== this.generation) return;
        const native: Record<string, unknown> = { step };
        const reads = { usage: this.native.usage, model: this.native.model, cwd: this.native.cwd };
        for (const [key, read] of Object.entries(reads)) {
          try {
            native[key] = await read();
          } catch {
            /* unknown */
          }
        }
        const request: StateRequest = {
          loadId: this.loadId,
          sequence: ++this.sequence,
          conversation: await this.native.id(),
          native,
          ...state,
        };
        await this.post(request);
      })
      .catch(() => {
        /* A later native event or heartbeat samples again. */
      });
  }

  // post sends a sample until the host takes it. A handover refuses one
  // between its snapshot and the exec with a 503, and the next image takes
  // it: the step's usage it carries may be replaced before the next sample.
  // Later samples wait behind it, in order; one of a generation the load no
  // longer holds is let go, and so is one still refused after 15 s.
  private async post(request: StateRequest) {
    for (let delay = nextDelay(0); ; delay = nextDelay(delay)) {
      const answer = await this.host.post(paths.modState, request);
      if (answer.status !== 503 || request.generation !== this.generation) return;
      if (delay === lastRetryMs) return;
      await new Promise<void>((resolve) => this.host.after(delay, resolve));
    }
  }

  compactStarted() {
    this.compacting = true;
    this.changed();
  }

  compactEnded(completed: boolean) {
    this.compacting = false;
    if (completed) {
      this.compactions++;
      this.last = { request: this.remote?.id ?? "", from: this.remote?.from ?? "" };
    }
    this.changed();
  }

  // A plugin-origin compaction is observed here through its returned result;
  // its event skips this plugin's hook, so it is counted once.
  async control(command: ControlCommand, generation: number) {
    if (generation !== this.generation || this.seen.has(command.id)) return;
    this.seen.add(command.id);
    if (this.seen.size > 64) this.seen.delete(this.seen.values().next().value!);
    const refuse = (code: ControlCode) =>
      this.reply(command, generation, { outcome: "refused", code });
    if (command.conversation !== (await this.native.id())) return refuse("conversation");
    if (command.action === "trace" && command.text !== undefined) {
      try {
        const result = await this.native.trace(command.text);
        if (result && typeof result === "object" && "deny" in result)
          return this.reply(command, generation, { outcome: "failed", native: result });
        return this.reply(command, generation, { outcome: "done" });
      } catch (error) {
        return this.reply(command, generation, { outcome: "failed", native: thrown(error) });
      }
    }
    if (this.compacting || this.remote || this.interrupting) return refuse("busy");
    if (command.action === "prompt" && command.text !== undefined) {
      // Submitted from the clock, outside the heartbeat's dispatch: a
      // plugin's prompt runs once the session is idle, and a turn may be
      // going on now.
      const text = command.text;
      this.host.after(0, () => {
        void this.native.submit(text).catch(() => undefined);
      });
      return this.reply(command, generation, { outcome: "done" });
    }
    if (command.action === "compact") {
      if (this.turn) return refuse("turn");
      this.remote = command;
      this.compactStarted();
      await this.reply(command, generation, { outcome: "requested" });
      try {
        // The result as resolved, less the transcript it carries.
        const { skip, tokensBefore, tokensAfter } = await this.native.compact(command.focus);
        this.compactEnded(skip === undefined);
        await this.reply(command, generation, {
          outcome: "done",
          native: { skip, tokensBefore, tokensAfter },
        });
      } catch (error) {
        this.compactEnded(false);
        await this.reply(command, generation, { outcome: "failed", native: thrown(error) });
      } finally {
        this.remote = undefined;
      }
      return;
    }
    if (command.action === "interrupt") {
      const turn = this.turn;
      if (!turn) return refuse("idle");
      if (command.turn !== undefined && command.turn !== turn) return refuse("turn");
      this.interrupting = turn;
      try {
        await this.native.abort(turn);
      } catch (error) {
        if (this.interrupting === turn) this.interrupting = undefined;
        return this.reply(command, generation, { outcome: "failed", native: thrown(error) });
      }
      return this.reply(command, generation, { outcome: "requested" });
    }
    await this.reply(command, generation, { outcome: "refused" });
  }

  private async reply(command: ControlCommand, generation: number, result: Result) {
    try {
      await this.host.post(paths.modControl, {
        loadId: this.loadId,
        generation,
        id: command.id,
        ...result,
      } satisfies ControlReply);
    } catch {
      /* The host reports a missing acknowledgement as unknown. */
    }
  }
}
