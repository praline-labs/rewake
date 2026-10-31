// The strip, the row under a pane, main's and each worker's (docs/shell.md,
// "The strip"): who the session is, what it does, where it works, its model
// and how much of its context it used, from rewake's own records, the same for every
// harness.
import { RGBA } from "@opentui/core";
import { createSignal, For, onCleanup } from "solid-js";
import { ShellRow, selecting, separator } from "./bar.tsx";
import { type FaceMode, modeOf } from "./face/faces.ts";
import type { Session, Space, Verdict } from "./room.ts";
import type { Theme } from "./theme.ts";

export type StripState = {
  session: string;
  branch?: string;
  status?: Session;
  // face is the pane's face where its session serves a mirror: the dev mark.
  face?: FaceMode;
};

// The room as last read: the spaces not over, and the sessions as main sees
// them.
const [spaces, showSpaces] = createSignal<Space[]>([]);
const [sessions, showSessions] = createSignal(new Map<string, Session>());

export { showSessions, showSpaces };

// stripOf is a worker pane's state: its space's session, as main sees it,
// and the branch of the space's own checkout, else of where it started.
export function stripOf(id: string): StripState {
  const space = spaces().find((s) => s.id === id);
  const session = space?.worker ?? id;
  const status = sessions().get(session);
  return { session, status, branch: space?.checkout?.branch ?? status?.branch, face: modeOf(id) };
}

// mainStrip is main's own: its row of the listing, once the run the listing
// is asked as is known, and the branch where it started.
export function mainStrip(): StripState {
  const status = [...sessions().values()].find((s) => s.own);
  return { session: status?.name ?? "main", status, branch: status?.branch };
}

type Part = {
  text: string;
  kind: "name" | "plain" | "context" | "cache" | "dev";
  fill?: number;
  cold?: boolean;
};

// How often the remaining life of the cache is counted again from the clock:
// the cell moves by the minute, and the poll is slower than that.
const tick = 5000;

// tokens is a count as the owner's own status line writes it: 950, 230k, 1M,
// 1.5M.
export function tokens(n: number): string {
  if (n >= 1_000_000) return `${Math.round(n / 100_000) / 10}M`;
  if (n >= 1000) return `${Math.round(n / 1000)}k`;
  return String(n);
}

// activityOf is the verdict's label, as rewake list's Status has it; an
// unknown activity says nothing in a narrow line.
function activityOf(s: Session): string | undefined {
  if (!s.verdict || s.verdict.activity === "unknown") return undefined;
  return s.verdict.label;
}

// Mark is what a worker's button edge says (docs/shell.md, "The bar"), as
// rewake judged it.
export type Mark = Verdict["mark"];

// markOf is the verdict's mark; a session not listed, or with no telemetry,
// is off.
export function markOf(s: Session | undefined): Mark {
  return s?.verdict?.mark ?? "off";
}

// windowOf is the context as the strip shows it, used of the window: 145k/1M,
// –/1M before the harness counted it.
export function windowOf(s: Session | undefined): string | undefined {
  if (!s?.window) return undefined;
  return `${s.used === undefined ? "–" : tokens(s.used)}/${tokens(s.window)}`;
}

// contextOf is the context's part, in the colour of how full it is.
function contextOf(s: Session): Part | undefined {
  const text = windowOf(s);
  if (!text) return undefined;
  return { text, kind: "context", fill: s.used === undefined ? undefined : s.percent };
}

// cacheOf is the cache as `rewake list` shows it: the hit percent and, where
// the harness says when the cache expires, what is left on the shell's own
// clock — "42m", "<1m", "—" once expired.
export function cacheOf(s: Session, now: number): Part | undefined {
  if (!s.cache) return undefined;
  let text = `${s.cache.hit}%`;
  let cold = false;
  if (s.cache.expiresAt !== undefined) {
    const left = s.cache.expiresAt - now;
    cold = left <= 0;
    text += cold ? " —" : left < 60_000 ? " <1m" : ` ${Math.ceil(left / 60_000)}m`;
  }
  return { text, kind: "cache", cold };
}

// nameOf is who the session is, with its type after it.
export function nameOf(state: StripState): string {
  const type = state.status?.type;
  return type ? `${state.session} (${type})` : state.session;
}

// stripParts is the line for a width: the name, activity and context always,
// the model, then the cache, then the branch left out while the line is too
// long, and the rest cut at the width.
export function stripParts(state: StripState, width: number, now = Date.now()): Part[] {
  const s = state.status;
  const activity = s && activityOf(s);
  const cache = s && cacheOf(s, now);
  // In the order shown; the optional ones are left out, 1 then 2 then 3.
  const all: ((Part & { optional?: number }) | undefined)[] = [
    { text: nameOf(state), kind: "name" },
    state.face ? { text: `dev ${state.face}`, kind: "dev" } : undefined,
    activity ? { text: activity, kind: "plain" } : undefined,
    state.branch ? { text: `⎇ ${state.branch}`, kind: "plain", optional: 3 } : undefined,
    // Thinking is named only while it is on: off and unknown say nothing.
    s?.model
      ? {
          text: [s.model, s.effort, s.thinking ? "thinking" : undefined].filter(Boolean).join(" "),
          kind: "plain",
          optional: 1,
        }
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

// fillColour is the context's colour by how full it is, the owner's own
// thresholds: the terminal's own green, yellow and red.
function fillColour(fill: number | undefined): RGBA {
  if (fill === undefined) return RGBA.defaultForeground();
  return RGBA.fromIndex(fill >= 80 ? 1 : fill >= 50 ? 3 : 2);
}

// Strip is the strip of a pane width wide, its name in the colour of the
// pane's program: a shell's row like the bar, its line fitted to the width
// less the padding.
export function Strip(props: {
  state: () => StripState;
  width: number;
  colour: string;
  theme: Theme;
  pick: () => void;
}) {
  const plain = RGBA.defaultForeground();
  const [now, setNow] = createSignal(Date.now());
  const timer = setInterval(() => setNow(Date.now()), tick);
  onCleanup(() => clearInterval(timer));
  const parts = () => stripParts(props.state(), Math.max(props.width - 2, 1), now());
  return (
    <ShellRow theme={props.theme} onMouseDown={props.pick}>
      <text height={1} wrapMode="none" flexGrow={1} fg={plain} {...selecting(props.theme)}>
        <For each={parts()}>
          {(part, i) => (
            <>
              {i() > 0 ? <span style={{ fg: props.theme.faint }}>{separator}</span> : ""}
              {part.kind === "name" ? (
                <b style={{ fg: props.colour }}>{part.text}</b>
              ) : part.kind === "context" ? (
                <span style={{ fg: fillColour(part.fill) }}>{part.text}</span>
              ) : part.kind === "dev" ? (
                <span style={{ fg: RGBA.fromIndex(5) }}>{part.text}</span>
              ) : part.kind === "cache" ? (
                <span style={{ fg: part.cold ? props.theme.faint : RGBA.fromIndex(6) }}>
                  {part.text}
                </span>
              ) : (
                part.text
              )}
            </>
          )}
        </For>
      </text>
    </ShellRow>
  );
}
