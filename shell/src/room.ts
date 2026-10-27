// The room, from one call of rewake's own polled once a second
// (docs/shell.md, "The prefix and the room"): `rewake room --json`, a hidden
// command made for the shell, answers the spaces, the sessions with their
// verdicts and, while the briefs show, the briefs. Its shape is the schema
// internal/cli/room_view.schema.json, from which room_view.gen.ts and the
// command's Go types are generated, so the shell maps nothing by hand.
import type { RoomView, SessionRow, SpaceRow } from "./room_view.gen.ts";

export type {
  BriefBound,
  BriefComment,
  BriefView,
  Problem,
  RoomView,
  Verdict,
} from "./room_view.gen.ts";
export type Space = SpaceRow;
export type Session = SessionRow;

const every = 1000;

// What the next calls ask beyond the spaces and the sessions: the briefs
// while their view shows, and the brief opened in it.
export type Want = { briefs: boolean; brief?: number };

let want: Want = { briefs: false };
export const wanting = (next: Want) => {
  want = next;
};

// One main lives in a room at a time, so the spaces whose main runs are this
// shell's main's.
export const shownSpaces = (view: RoomView): Space[] =>
  view.spaces.filter((s) => s.mainRun === "running");

// endedSpaces is the spaces not over whose main's run reads ended: the first
// sign of main's end the room gives, while its hosts end the workers.
export const endedSpaces = (view: RoomView): string[] =>
  view.spaces.filter((s) => s.mainRun === "ended").map((s) => s.id);

// readRoom is the room view, or undefined when it does not read. mainPid is
// the pid of main's wrapper once it runs: the command proves from it that the
// caller is main's shell, and only then carries telemetry and briefs.
export async function readRoom(
  rewake: string,
  room: string,
  mainPid?: number,
  asked: Want = { briefs: false },
): Promise<RoomView | undefined> {
  const args = ["room", "--json"];
  if (mainPid !== undefined) args.push("--wrapper", String(mainPid));
  if (asked.briefs) args.push("--briefs");
  if (asked.brief !== undefined) args.push("--brief", String(asked.brief));
  try {
    const child = Bun.spawn([rewake, ...args], {
      env: { ...(process.env as Record<string, string>), REWAKE_ROOM: room },
      stdout: "pipe",
      stderr: "ignore",
      stdin: "ignore",
    });
    const text = await new Response(child.stdout).text();
    if ((await child.exited) === 0) return JSON.parse(text) as RoomView;
  } catch {
    // Read as no view.
  }
  return undefined;
}

// pollRoom calls onRead with each view that reads, and with undefined when
// one does not, together with what that call asked for. It returns the stop.
export function pollRoom(
  rewake: string,
  room: string,
  mainPid: () => number | undefined,
  onRead: (view: RoomView | undefined, asked: Want) => void,
): () => void {
  let stopped = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const once = async () => {
    const asked = want;
    const view = await readRoom(rewake, room, mainPid(), asked);
    if (!stopped) onRead(view, asked);
    if (!stopped) timer = setTimeout(once, every);
  };
  void once();
  return () => {
    stopped = true;
    clearTimeout(timer);
  };
}

// spacesOver resolves once none of the spaces ids is open or closing in the
// room's view — each worker ended by its host after main's run ended — or
// once bound passes: a space host takes up to about nine seconds to end a
// worker that ignores its hangup and SIGTERM.
export async function spacesOver(rewake: string, room: string, ids: string[], bound: number) {
  const until = Date.now() + bound;
  while (ids.length > 0 && Date.now() < until) {
    const view = await readRoom(rewake, room);
    // A view that does not read shows nothing to wait for.
    if (!view) return;
    const live = view.spaces.filter(
      (s) => ids.includes(s.id) && (s.state === "open" || s.state === "closing"),
    );
    if (live.length === 0) return;
    await new Promise((resolve) => setTimeout(resolve, 200));
  }
}
