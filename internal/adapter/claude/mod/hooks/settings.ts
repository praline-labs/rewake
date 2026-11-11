// The run's status line in the settings Claude Code reads
// (docs/claude.md#the-cache-ttl). A launch puts rewake's statusLine into the
// run's --settings, but the flag is single-use (docs/research-launch.md): a
// --command wrapper that adds a --settings of its own, or a person who passes
// one, ends the read's last layer there, and the person's own status line comes
// back in the run with rewake's gone. The launch names the same setting in
// REWAKE_STATUS_LINE for this hook, which puts it into the settings the engine
// runs under: every other key of the read stays as it came, the wrapper's own
// included, so nothing of the wrapper's is lost. A managed statusLine is the
// administrator's and outranks a launch's, so it is left alone.
import type { Register } from "claude-code";

// serveSettings keeps the launch's status line in the settings Claude Code
// reads, on the engine's own read and on a mod's read of the merged settings.
export function serveSettings(on: Parameters<Register>[0]) {
  on("settings.read", async ($, e, next) => {
    const read = await next(e);
    // A read of one source is that source's settings, not the session's
    // merged ones, and a read a later hook refused has no value to rewrite.
    if (e.source !== undefined || !read.value) return read;
    if (managedStatusLine(e.layers)) return read;
    // The variable is spelled out: the engine lists the names a module
    // reads, so $.env.get takes a literal.
    const carried = await $.env.get("REWAKE_STATUS_LINE");
    const kept = withStatusLine(read.value, carried);
    // Nothing to add: the read is given back as it came.
    return kept === read.value ? read : { value: kept };
  });
}

// withStatusLine is the settings read with the launch's statusLine, or the read
// itself when the run's launch carried none, or carried no setting.
function withStatusLine(
  read: Readonly<Record<string, unknown>>,
  carried: string | undefined,
): Readonly<Record<string, unknown>> {
  if (!carried) return read;
  let line: unknown;
  try {
    line = JSON.parse(carried);
  } catch {
    return read;
  }
  if (line === null || typeof line !== "object") return read;
  return { ...read, statusLine: line };
}

// managedStatusLine says an administrator set a status line: the policy layer
// outranks every other layer, so the engine's own is the one to keep.
function managedStatusLine(
  layers: readonly { source: string; settings: Readonly<Record<string, unknown>> }[],
): boolean {
  return layers.some(
    (layer) => layer.source === "policy" && layer.settings.statusLine !== undefined,
  );
}
