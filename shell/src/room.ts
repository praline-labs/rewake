// The room, from rewake's own commands polled once a second
// (docs/v2/shell.md, "The prefix and the room"): the spaces of the room's
// running main, each with the socket its host serves the worker's screen on.

export type Space = {
  id: string;
  name: string;
  worker: string;
  socket: string;
  // checkout is the worker's own worktree: the one its launch made, or the
  // one it entered; absent while it works where main does.
  checkout?: { path?: string; branch?: string };
};

const every = 1000;

type Listing = {
  spaces?: {
    id: string;
    name: string;
    state: string;
    mainRun?: string;
    socket?: string;
    worker?: { name?: string; harness?: string };
    checkout?: { path?: string; branch?: string };
  }[];
};

// One main lives in a room at a time, so the spaces whose main runs are this
// shell's main's.
function spacesOf(listing: Listing): Space[] {
  return (listing.spaces ?? [])
    .filter(
      (s) => s.mainRun === "running" && s.socket && s.state !== "kept" && s.state !== "closed",
    )
    .map((s) => ({
      id: s.id,
      name: s.name,
      worker: `${s.worker?.name ?? s.name}-${s.worker?.harness ?? ""}`,
      socket: s.socket!,
      checkout: s.checkout?.path ? s.checkout : undefined,
    }));
}

// endedOf is the spaces not over whose main's run reads ended: the first
// sign of main's end the room gives, while its hosts end the workers.
function endedOf(listing: Listing): string[] {
  return (listing.spaces ?? [])
    .filter((s) => s.mainRun === "ended" && s.state !== "kept" && s.state !== "closed")
    .map((s) => s.id);
}

// readRewake is a view of rewake's in the room, read as JSON, or undefined
// when it does not read; as names the session it is asked for.
export async function readRewake<T>(
  rewake: string,
  room: string,
  args: string[],
  as?: Record<string, string>,
): Promise<T | undefined> {
  try {
    const child = Bun.spawn([rewake, ...args], {
      env: { ...(process.env as Record<string, string>), REWAKE_ROOM: room, ...as },
      stdout: "pipe",
      stderr: "ignore",
      stdin: "ignore",
    });
    const text = await new Response(child.stdout).text();
    if ((await child.exited) === 0) return JSON.parse(text);
  } catch {
    // Read as no listing.
  }
  return undefined;
}

// readListing is the room's spaces, or undefined when they do not read.
function readListing(rewake: string, room: string): Promise<Listing | undefined> {
  return readRewake<Listing>(rewake, room, ["space", "list", "--json"]);
}

// pollRoom calls onSpaces with each listing of the spaces that reads, and
// onSessions with each of the sessions; a failed read keeps the last. It
// returns the stop.
export function pollRoom(
  rewake: string,
  room: string,
  onSpaces: (spaces: Space[], ended: string[]) => void,
  mainPid: () => number | undefined,
  onSessions: (sessions: Map<string, Session>) => void,
): () => void {
  let stopped = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const readSessions = sessionReader(rewake, room, mainPid);
  const once = async () => {
    const [listing, sessions] = await Promise.all([readListing(rewake, room), readSessions()]);
    if (listing && !stopped) onSpaces(spacesOf(listing), endedOf(listing));
    if (sessions && !stopped) onSessions(sessions);
    if (!stopped) timer = setTimeout(once, every);
  };
  void once();
  return () => {
    stopped = true;
    clearTimeout(timer);
  };
}

// spacesOver resolves once none of the spaces ids is open or closing in the
// room's listing — each worker ended by its host after main's run ended — or
// once bound passes: a space host takes up to about nine seconds to end a
// worker that ignores its hangup and SIGTERM.
export async function spacesOver(rewake: string, room: string, ids: string[], bound: number) {
  const until = Date.now() + bound;
  while (ids.length > 0 && Date.now() < until) {
    const listing = await readListing(rewake, room);
    // A listing that does not read shows nothing to wait for.
    if (!listing) return;
    const live = (listing.spaces ?? []).filter(
      (s) => ids.includes(s.id) && (s.state === "open" || s.state === "closing"),
    );
    if (live.length === 0) return;
    await new Promise((resolve) => setTimeout(resolve, 200));
  }
}

// The room's sessions as main sees them, for the strip under each worker pane
// (docs/v2/shell.md, "The strip"): rewake list shows a session's telemetry
// only to a verified main, and the shell is main's terminal, so it asks as
// main's run, the one whose wrapper is its own child.
export type Session = {
  name: string;
  type?: string;
  cwd: string;
  // branch is what is checked out where the session started.
  branch?: string;
  // activity is idle, working, not_loaded or system_error; fresh is false
  // when the run stopped publishing, and every value is then its last.
  activity?: string;
  waitingFor?: string[];
  // dialog is set while the session waits at a dialog on its screen that no
  // request carries to main.
  dialog?: string;
  compacting?: boolean;
  fresh: boolean;
  model?: string;
  effort?: string;
  used?: number;
  window?: number;
  percent?: number;
};

type Telemetry = {
  activity?: string | null;
  waitingFor?: string[] | null;
  dialog?: string | null;
  fresh?: boolean;
  compactionInProgress?: boolean | null;
  configuredModel?: string | null;
  configuredModelName?: string | null;
  configuredReasoningEffort?: string | null;
  contextUsedTokens?: number | null;
  contextWindowTokens?: number | null;
  contextFilledPercent?: number | null;
};

type SessionListing = {
  sessions?: {
    name: string;
    role: string;
    cwd: string;
    type?: string;
    servicePid: number;
    serviceStart: number;
    boot?: string;
    telemetry?: Telemetry;
  }[];
};

// epochOf is a run's epoch as rewake names it (registry.RunEpoch).
function epochOf(s: { servicePid: number; serviceStart: number; boot?: string }): string {
  return `${s.servicePid}.${s.serviceStart}${s.boot ? `.${s.boot}` : ""}`;
}

function sessionsOf(listing: SessionListing): Map<string, Session> {
  const out = new Map<string, Session>();
  for (const s of listing.sessions ?? []) {
    const t = s.telemetry;
    out.set(s.name, {
      name: s.name,
      type: s.type,
      cwd: s.cwd,
      activity: t?.activity ?? undefined,
      waitingFor: t?.waitingFor ?? undefined,
      dialog: t?.dialog ?? undefined,
      compacting: t?.compactionInProgress ?? undefined,
      fresh: t?.fresh ?? false,
      model: t?.configuredModelName ?? t?.configuredModel ?? undefined,
      effort: t?.configuredReasoningEffort ?? undefined,
      used: t?.contextUsedTokens ?? undefined,
      window: t?.contextWindowTokens ?? undefined,
      percent: t?.contextFilledPercent ?? undefined,
    });
  }
  return out;
}

// sessionReader reads the room's sessions. mainPid is the pid of main's
// wrapper once it runs: its record, found by that pid, names the run the
// listing is asked as; until then the listing has no telemetry.
export function sessionReader(rewake: string, room: string, mainPid: () => number | undefined) {
  let as: { REWAKE_SESSION: string; REWAKE_EPOCH: string } | undefined;
  const read = async (again = true): Promise<Map<string, Session> | undefined> => {
    const listing = await readRewake<SessionListing>(rewake, room, ["list", "--json"], as);
    if (!listing) return undefined;
    const pid = mainPid();
    const own = listing.sessions?.find((s) => s.role === "main" && s.servicePid === pid);
    const was = as?.REWAKE_EPOCH;
    as = own ? { REWAKE_SESSION: own.name, REWAKE_EPOCH: epochOf(own) } : undefined;
    // A listing asked as no run, or another, has no telemetry: asked again
    // at once as main's.
    if (as && as.REWAKE_EPOCH !== was && again) return read(false);
    const sessions = sessionsOf(listing);
    for (const s of sessions.values()) s.branch = await branchOf(s.cwd);
    return sessions;
  };
  return read;
}

const branchEvery = 5000;

// branches keeps the branch checked out in each directory a session started
// in, read from git at most every five seconds: a worker without a checkout
// of its own works on what is checked out there.
const branches = new Map<string, { at: number; branch?: string }>();

async function git(cwd: string, ...args: string[]): Promise<string | undefined> {
  try {
    const child = Bun.spawn(["git", "-C", cwd, ...args], {
      stdout: "pipe",
      stderr: "ignore",
      stdin: "ignore",
    });
    const text = (await new Response(child.stdout).text()).trim();
    if ((await child.exited) === 0 && text) return text;
  } catch {
    // No git: no branch shown.
  }
  return undefined;
}

// branchOf is the branch checked out in cwd, a detached head's short commit,
// or undefined outside a repository.
async function branchOf(cwd: string): Promise<string | undefined> {
  const known = branches.get(cwd);
  if (known && Date.now() - known.at < branchEvery) return known.branch;
  let branch = await git(cwd, "rev-parse", "--abbrev-ref", "HEAD");
  if (branch === "HEAD") branch = await git(cwd, "rev-parse", "--short", "HEAD");
  branches.set(cwd, { at: Date.now(), branch });
  return branch;
}
