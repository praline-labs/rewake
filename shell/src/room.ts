// The room, from rewake's own commands polled once a second
// (docs/v2/shell.md, "The prefix and the room"): the spaces of the room's
// running main, each with the socket its host serves the worker's screen on.

export type Space = { id: string; name: string; worker: string; socket: string };

const every = 1000;

type Listing = {
  spaces?: {
    id: string;
    name: string;
    state: string;
    mainRun?: string;
    socket?: string;
    worker?: { name?: string; harness?: string };
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
    }));
}

// endedOf is the spaces not over whose main's run reads ended: the first
// sign of main's end the room gives, while its hosts end the workers.
function endedOf(listing: Listing): string[] {
  return (listing.spaces ?? [])
    .filter((s) => s.mainRun === "ended" && s.state !== "kept" && s.state !== "closed")
    .map((s) => s.id);
}

// pollRoom calls onSpaces with each listing that reads; a failed read keeps
// the last. It returns the stop.
export function pollRoom(
  rewake: string,
  room: string,
  onSpaces: (spaces: Space[], ended: string[]) => void,
): () => void {
  let stopped = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const env: Record<string, string> = {
    ...(process.env as Record<string, string>),
    REWAKE_ROOM: room,
  };
  const once = async () => {
    try {
      const child = Bun.spawn([rewake, "space", "list", "--json"], {
        env,
        stdout: "pipe",
        stderr: "ignore",
        stdin: "ignore",
      });
      const text = await new Response(child.stdout).text();
      if ((await child.exited) === 0 && !stopped) {
        const listing = JSON.parse(text);
        onSpaces(spacesOf(listing), endedOf(listing));
      }
    } catch {
      // The next poll tries again.
    }
    if (!stopped) timer = setTimeout(once, every);
  };
  void once();
  return () => {
    stopped = true;
    clearTimeout(timer);
  };
}
