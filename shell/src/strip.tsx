// The strip, the last row of a worker's pane (docs/v2/shell.md, "The
// strip"): who the worker is, what it does, where it works, its model and how
// full its context is, from rewake's own records, the same for every harness.
import { RGBA, TextAttributes } from "@opentui/core";
import { createSignal, For, onCleanup } from "solid-js";
import type { Session, Space } from "./room.ts";
import type { Theme } from "./theme.ts";

export type StripState = { session: string; branch?: string; status?: Session };

// The room as last read: the spaces not over, and the sessions as main sees
// them.
const [spaces, showSpaces] = createSignal<Space[]>([]);
const [sessions, showSessions] = createSignal(new Map<string, Session>());

export { showSessions, showSpaces };

// stripOf is a worker pane's state: its space's session, as main sees it,
// and the branch of the space's own checkout, else of where it started.
function stripOf(id: string): StripState {
  const space = spaces().find((s) => s.id === id);
  const session = space?.worker ?? id;
  const status = sessions().get(session);
  return { session, status, branch: space?.checkout?.branch ?? status?.branch };
}

type Part = { text: string; kind: "name" | "plain" | "context"; fill?: number };

// How often the remaining life of the cache is counted again from the clock:
// the cell moves by the minute, and the poll is slower than that.
const tick = 5000;

const separator = " · ";

// tokens is a count as the owner's own status line writes it: 950, 230k, 1M,
// 1.5M.
export function tokens(n: number): string {
  if (n >= 1_000_000) return `${Math.round(n / 100_000) / 10}M`;
  if (n >= 1000) return `${Math.round(n / 1000)}k`;
  return String(n);
}

function activityOf(s: Session): string | undefined {
  let text: string | undefined;
  if (s.activity === "working")
    text = s.waitingFor?.length ? `waiting for ${s.waitingFor.join(" and ")}` : "working";
  else if (s.activity === "not_loaded") text = "not loaded";
  else if (s.activity === "system_error") text = "system error";
  else text = s.activity;
  if (s.compacting) text = text ? `${text}, compacting` : "compacting";
  if (text && !s.fresh) text += " (stale)";
  return text;
}

function contextOf(s: Session): Part | undefined {
  if (!s.window) return undefined;
  if (s.used === undefined || s.percent === undefined)
    return { text: `–/${tokens(s.window)}`, kind: "context" };
  return {
    text: `${s.percent}% ${tokens(s.used)}/${tokens(s.window)}`,
    kind: "context",
    fill: s.percent,
  };
}

// cacheOf is the cache as `rewake list` shows it: the hit percent and, where
// the harness says when the cache expires, what is left on the shell's own
// clock — "42m", "<1m", "—" once expired.
export function cacheOf(s: Session, now: number): Part | undefined {
  if (!s.cache) return undefined;
  let text = `${s.cache.hit}%`;
  if (s.cache.expiresAt !== undefined) {
    const left = s.cache.expiresAt - now;
    text += left <= 0 ? " —" : left < 60_000 ? " <1m" : ` ${Math.ceil(left / 60_000)}m`;
  }
  return { text, kind: "plain" };
}

// stripParts is the line for a width: the name, activity and context always,
// the model, then the cache, then the branch left out while the line is too
// long, and the rest cut at the width.
export function stripParts(state: StripState, width: number, now = Date.now()): Part[] {
  const s = state.status;
  const name = s?.type ? `${state.session} (${s.type})` : state.session;
  const activity = s && activityOf(s);
  const cache = s && cacheOf(s, now);
  // In the order shown; the optional ones are left out, 1 then 2 then 3.
  const all: ((Part & { optional?: number }) | undefined)[] = [
    { text: name, kind: "name" },
    activity ? { text: activity, kind: "plain" } : undefined,
    state.branch ? { text: `⎇ ${state.branch}`, kind: "plain", optional: 3 } : undefined,
    s?.model
      ? { text: [s.model, s.effort].filter(Boolean).join(" "), kind: "plain", optional: 1 }
      : undefined,
    s ? contextOf(s) : undefined,
    cache ? { ...cache, optional: 2 } : undefined,
  ];
  let parts = all.filter((p) => p !== undefined);
  const length = () =>
    parts.reduce((n, p) => n + p.text.length, 0) + separator.length * (parts.length - 1);
  for (const drop of [1, 2, 3])
    if (length() > width) parts = parts.filter((p) => p.optional !== drop);
  // What still does not fit is cut at the width.
  let room = width;
  const out: Part[] = [];
  for (const [i, part] of parts.entries()) {
    const sep = i > 0 ? separator.length : 0;
    if (room - sep <= 0) break;
    const text =
      part.text.length > room - sep ? `${part.text.slice(0, room - sep - 1)}…` : part.text;
    out.push({ ...part, text });
    room -= sep + text.length;
  }
  return out;
}

// indent is the spaces that centre the parts in the width; none once they
// fill it.
export function indent(parts: Part[], width: number): string {
  const length =
    parts.reduce((n, p) => n + p.text.length, 0) + separator.length * (parts.length - 1);
  return " ".repeat(Math.max(0, Math.floor((width - length) / 2)));
}

// fillColour is the context's colour by how full it is, the owner's own
// thresholds: the terminal's own green, yellow and red.
function fillColour(fill: number | undefined): RGBA {
  if (fill === undefined) return RGBA.defaultForeground();
  return RGBA.fromIndex(fill >= 80 ? 1 : fill >= 50 ? 3 : 2);
}

// Strip is the strip of the space id's worker.
export function Strip(props: { id: string; width: number; theme: Theme }) {
  const plain = RGBA.defaultForeground();
  const [now, setNow] = createSignal(Date.now());
  const timer = setInterval(() => setNow(Date.now()), tick);
  onCleanup(() => clearInterval(timer));
  const parts = () => stripParts(stripOf(props.id), props.width, now());
  return (
    <text height={1} width={props.width} wrapMode="none" fg={plain}>
      {indent(parts(), props.width)}
      <For each={parts()}>
        {(part, i) => (
          <>
            {i() > 0 ? (
              <span style={{ fg: plain, attributes: TextAttributes.DIM }}>{separator}</span>
            ) : (
              ""
            )}
            {part.kind === "name" ? (
              <b style={{ fg: props.theme.worker }}>{part.text}</b>
            ) : part.kind === "context" ? (
              <span style={{ fg: fillColour(part.fill) }}>{part.text}</span>
            ) : (
              part.text
            )}
          </>
        )}
      </For>
    </text>
  );
}
