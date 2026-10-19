// The load's heartbeat (docs/v2/stage4-turns.md#the-mods-side): one held
// mod/alive poll at a time for the newest generation a hello of this load was
// given, each sent from a timer's callback once the one before is answered,
// so the chain keeps going between turns and a reload cancels it with the
// old copy. The host withdraws what the mod proved when the polls stop.

import type { AliveAnswer, AliveRequest } from "../protocol.gen.ts";
import { paths } from "../protocol.gen.ts";
import type { Host } from "./outbox.ts";
import { nextDelay } from "./outbox.ts";

export class Heartbeat {
  // newest is the newest generation a hello of this load answered: a chain
  // of an older one ends rather than run beside it.
  private newest = 0;
  // superseded is set by a 410: a later load holds the run, and this one
  // says no hello again.
  superseded = false;

  constructor(
    private readonly host: Host,
    private readonly loadId: string,
    // again says the load's hello once more, for a generation that lapsed.
    private readonly again: () => void,
    private readonly received?: (answer: AliveAnswer, generation: number) => void,
  ) {}

  // begin runs a chain for a generation a hello answered; a retried hello
  // answered the generation whose chain runs already, and starts none.
  begin(generation: number) {
    if (this.superseded || generation <= this.newest) {
      return;
    }
    this.newest = generation;
    this.host.after(0, () => {
      void this.poll(generation, 0);
    });
  }

  // poll sends one poll and schedules what its answer calls for: the next
  // poll at once after a 200; a hello again after a 409 of the newest
  // generation; nothing after a 410, a 409 of an older generation or another
  // refusal; the same poll after 1 s, doubling up to 10 s, after a throw, a
  // 500 or a 503.
  private async poll(generation: number, delay: number) {
    if (this.superseded || generation !== this.newest) {
      return;
    }
    const request: AliveRequest = { loadId: this.loadId, generation };
    let status = 0;
    let text = "";
    try {
      const answer = await this.host.post(paths.modAlive, request);
      status = answer.status;
      text = answer.text;
    } catch {
      // No answer: sent again, below.
    }
    if (this.superseded || generation !== this.newest) {
      return;
    }
    switch (status) {
      case 200:
        try {
          this.received?.(JSON.parse(text) as AliveAnswer, generation);
        } catch {
          this.host.log("rewake: malformed heartbeat answer");
        }
        this.host.after(0, () => {
          void this.poll(generation, 0);
        });
        return;
      case 409:
        this.again();
        return;
      case 410:
        this.superseded = true;
        return;
      case 0:
      case 500:
      case 503: {
        const next = nextDelay(delay);
        this.host.after(next, () => {
          void this.poll(generation, next);
        });
        return;
      }
      default:
        this.host.log(
          `rewake: the heartbeat was refused (${status}) and stops, so the run withdraws what this load proved: ${text}`,
        );
    }
  }
}
