// Native state and control stay outside the heartbeat's awaited chain: a
// compaction may take minutes while the load must keep renewing its lease.

import type { ControlCommand, ControlReply, StateRequest } from "../protocol.gen.ts";
import { paths } from "../protocol.gen.ts";
import type { Host } from "./outbox.ts";

export interface Native {
  id: () => Promise<string>;
  model: () => Promise<string>;
  usage: () => Promise<{ context: { tokens?: number; window: number; percent?: number } }>;
  compact: (
    focus: string,
  ) => Promise<{ skip?: string; tokensBefore?: number; tokensAfter?: number }>;
  abort: (turnId: string) => Promise<void>;
  trace: (text: string) => Promise<unknown>;
}

type Result = Pick<ControlReply, "outcome"> & Partial<ControlReply>;

export class Management {
  private generation = 0;
  private sequence = 0;
  private compactions = 0;
  private last = { request: "", from: "" };
  private turn?: string;
  private effort?: string;
  private cache?: { cacheRead: number; cacheWrite: number; cacheInput: number };
  private compacting = false;
  private remote?: ControlCommand;
  private interrupt?: { command: ControlCommand; turn: string; generation: number };
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
  effortSeen(effort: string | number | undefined) {
    if (effort === undefined || String(effort) === this.effort) return;
    this.effort = String(effort);
    this.changed();
  }

  // The usage the API reported for a main-loop response is the cache's only
  // source; a response without usage keeps the last one's.
  usageSeen(
    usage: {
      cache_read_input_tokens?: number | null;
      cache_creation_input_tokens?: number | null;
      input_tokens?: number | null;
    } | null,
  ) {
    if (!usage) return;
    this.cache = {
      cacheRead: usage.cache_read_input_tokens ?? 0,
      cacheWrite: usage.cache_creation_input_tokens ?? 0,
      cacheInput: usage.input_tokens ?? 0,
    };
  }

  // Return the actor for the durable end, and leave the same trace in native
  // history without starting a model turn. Other turns cannot settle it.
  async ended(turnId: string, aborted: boolean): Promise<string | undefined> {
    if (this.turn === turnId) this.turn = undefined;
    const pending = this.interrupt;
    let actor: string | undefined;
    if (pending?.turn === turnId) {
      this.interrupt = undefined;
      if (aborted) {
        actor = `${pending.command.from} interrupted this turn with rewake interrupt`;
        try {
          const result = await this.native.trace(actor);
          if (result && typeof result === "object" && "deny" in result)
            throw new Error("trace refused");
          await this.reply(pending.command, pending.generation, { outcome: "done" });
        } catch {
          await this.reply(pending.command, pending.generation, {
            outcome: "failed",
            detail: "the turn stopped, but its interruption trace could not be recorded",
          });
        }
      } else {
        await this.reply(pending.command, pending.generation, {
          outcome: "refused",
          reason: "no turn running",
        });
      }
    }
    this.changed();
    return actor;
  }

  // Samples are serialized and complete: an unknown native usage clears an
  // older one instead of keeping pre-compaction tokens as current.
  changed() {
    if (!this.generation) return;
    const state = {
      generation: this.generation,
      activity: this.turn ? "working" : "idle",
      compacting: this.compacting,
      compactions: this.compactions,
      effort: this.effort,
      ...this.cache,
      ...this.last,
    };
    this.sending = this.sending
      .then(async () => {
        if (state.generation !== this.generation) return;
        let context: { tokens?: number; window?: number; percent?: number } = {};
        let model: string | undefined;
        try {
          context = (await this.native.usage()).context;
        } catch {
          /* unknown */
        }
        try {
          model = await this.native.model();
        } catch {
          /* unknown */
        }
        const request: StateRequest = {
          loadId: this.loadId,
          sequence: ++this.sequence,
          conversation: await this.native.id(),
          model,
          tokens: context.tokens,
          window: context.window,
          percent: context.percent,
          ...state,
        };
        await this.host.post(paths.modState, request);
      })
      .catch(() => {
        /* A later native event or heartbeat samples again. */
      });
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
    if (command.conversation !== (await this.native.id())) {
      await this.reply(command, generation, {
        outcome: "refused",
        reason: "not the selected conversation",
      });
      return;
    }
    if (this.compacting || this.remote || this.interrupt) {
      await this.reply(command, generation, {
        outcome: "refused",
        reason: "another request in flight",
      });
      return;
    }
    if (command.action === "compact") {
      if (this.turn) {
        await this.reply(command, generation, { outcome: "refused", reason: "in a turn" });
        return;
      }
      this.remote = command;
      this.compactStarted();
      await this.reply(command, generation, { outcome: "requested" });
      try {
        const result = await this.native.compact(command.focus);
        this.compactEnded(result.skip === undefined);
        await this.reply(
          command,
          generation,
          result.skip === undefined
            ? {
                outcome: "done",
                tokensBefore: result.tokensBefore,
                tokensAfter: result.tokensAfter,
              }
            : { outcome: "refused", detail: result.skip.slice(0, 300) },
        );
      } catch {
        this.compactEnded(false);
        await this.reply(command, generation, {
          outcome: "failed",
          reason: "cut short",
          detail: "native compaction failed or was interrupted",
        });
      } finally {
        this.remote = undefined;
      }
      return;
    }
    if (command.action === "interrupt") {
      if (!this.turn) {
        await this.reply(command, generation, { outcome: "refused", reason: "no turn running" });
        return;
      }
      const pending = { command, generation, turn: this.turn };
      this.interrupt = pending;
      this.host.after(3000, () => {
        if (this.interrupt === pending) {
          void this.reply(command, generation, {
            outcome: "failed",
            open: true,
            reason: "cut short",
            detail: "the turn has not settled after interrupt",
          });
        }
      });
      try {
        await this.native.abort(pending.turn);
      } catch {
        if (this.interrupt === pending) {
          this.interrupt = undefined;
          await this.reply(command, generation, {
            outcome: "failed",
            detail: "native interrupt could not be requested",
          });
        }
      }
      return;
    }
    await this.reply(command, generation, { outcome: "refused", detail: "unknown control action" });
  }

  private async reply(command: ControlCommand, generation: number, result: Result) {
    try {
      await this.host.post(paths.modControl, {
        loadId: this.loadId,
        generation,
        id: command.id,
        reason: "",
        detail: "",
        open: false,
        ...result,
      } satisfies ControlReply);
    } catch {
      /* The host reports a missing acknowledgement as unknown. */
    }
  }
}
