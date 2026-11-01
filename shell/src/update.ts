// What the shell knows of rewake's parts (docs/install.md, "The parts and
// what an update restarts"): the contracts this shell loaded, which it says
// in its hello and to `rewake-shell --contracts`, and the banner it shows
// when the room's plan, read with the room view, asks for a restart.
import { createSignal } from "solid-js";
import { contract } from "../../face/src/messages.ts";
import { bounds } from "../../face/src/mirror.gen.ts";
import type { Hint } from "./hints.ts";
import { requires } from "./link.ts";
import type { Available, UpdateStep } from "./room_view.gen.ts";

// contracts is the shell's and its face's, as a manifest records them.
export const contracts = () => ({
  shell: { requires: requires() },
  face: { speaks: { face: String(contract) }, requires: { mirror: String(bounds.version) } },
});

// helloContracts is what the hello says this shell loaded: its own
// contracts, and its face's when the face is compiled into it, since that
// face moves only with the shell (docs/shell.md, "The host's channel").
export const helloContracts = (embedded: boolean) => {
  const c = contracts();
  return embedded ? { requires: c.shell.requires, face: c.face } : { requires: c.shell.requires };
};

// bannerOf is the hint a plan gives the person, or none: a restart of the
// room before one of the shell, since it loads both; a part that reloads
// itself, or that new sessions take, asks nothing of the person. With no
// such step, a newer release on npm, with the most installing it would ask
// (docs/install.md, "Updating from inside rewake"); once it is installed the
// room view drops it, and the plan's own steps take over.
export function bannerOf(steps: UpdateStep[] | undefined, available?: Available): Hint | undefined {
  const room = (steps ?? []).filter((s) => s.class === "room").map((s) => s.part);
  if (room.length > 0) return { keys: "room restart needed:", does: room.join(", ") };
  const shell = (steps ?? []).filter((s) => s.class === "shell").map((s) => s.part);
  if (shell.length > 0) return { keys: "Ctrl+\\ r", does: `loads the updated ${shell.join(", ")}` };
  if (available) {
    return {
      keys: `${available.version} available`,
      does: `· restarts: ${available.restarts} · rewake update`,
    };
  }
  return undefined;
}

const [plan, setPlan] = createSignal<Hint | undefined>(undefined);
export const banner = plan;
export const takeUpdate = (steps: UpdateStep[] | undefined, available?: Available) => {
  const next = bannerOf(steps, available);
  const was = plan();
  if (next?.keys !== was?.keys || next?.does !== was?.does) setPlan(next);
};
