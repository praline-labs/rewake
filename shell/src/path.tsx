// The pane's top row (docs/shell.md, "The pane"): where its session works
// now, `~`-shortened, and the branch checked out there, from the room view's
// path and branch, the same for every harness. A worktree entry moves it; a
// `cd` in a shell command does not, as it moves no harness.
import { RGBA } from "@opentui/core";
import { ShellRow, selecting, separator } from "./bar.tsx";
import type { StripState } from "./strip.tsx";
import type { Theme } from "./theme.ts";

const graphemes = new Intl.Segmenter();
const cells = (text: string) => Bun.stringWidth(text);

// home is the person's home, which a path under it starts with `~` for.
const home = process.env.HOME ?? "";

// tilded is the path with the person's home as `~`.
export function tilded(path: string, under = home): string {
  if (!under || under === "/") return path;
  if (path === under) return "~";
  return path.startsWith(`${under}/`) ? `~${path.slice(under.length)}` : path;
}

// fromStart is the path in room cells: whole directories left out from its
// start, `…/` in their place, so it keeps those it ends with; a last one too
// long alone is cut from its start with `…`.
function fromStart(path: string, room: number): string {
  if (cells(path) <= room) return path;
  if (room < 1) return "";
  const dirs = path.split("/");
  for (let from = 1; from < dirs.length; from++) {
    const tail = `…/${dirs.slice(from).join("/")}`;
    if (cells(tail) <= room) return tail;
  }
  const kept = [...graphemes.segment(dirs.at(-1) ?? "")].map((g) => g.segment);
  while (kept.length > 0 && cells(`…${kept.join("")}`) > room) kept.shift();
  return `…${kept.join("")}`;
}

// fromEnd is the text in room cells, cut at its end with `…`.
function fromEnd(text: string, room: number): string {
  if (cells(text) <= room) return text;
  if (room < 1) return "";
  let kept = "";
  for (const { segment } of graphemes.segment(text)) {
    if (cells(`${kept + segment}…`) > room) break;
    kept += segment;
  }
  return `${kept}…`;
}

// pathParts is the row for a width: the path cut from its start where it is
// too long, then the branch; a row too narrow for both keeps the branch and
// cuts it at its end. Nothing before the path is known.
export function pathParts(
  state: Pick<StripState, "path" | "branch">,
  width: number,
  under = home,
): { path?: string; branch?: string } {
  if (!state.path) return {};
  const path = tilded(state.path, under);
  const branch = state.branch ? `⎇ ${state.branch}` : undefined;
  if (!branch) return { path: fromStart(path, width) };
  // At least `…` and the last letter of the path, the separator and the branch.
  const room = width - separator.length - cells(branch);
  if (room >= Math.min(cells(path), 2)) return { path: fromStart(path, room), branch };
  return { branch: fromEnd(branch, width) };
}

// PathRow is a pane's top row, width wide, fitted to it less the padding.
export function PathRow(props: {
  state: () => StripState;
  width: number;
  theme: Theme;
  pick: () => void;
}) {
  const parts = () => pathParts(props.state(), Math.max(props.width - 2, 1));
  return (
    <ShellRow theme={props.theme} onMouseDown={props.pick}>
      <text
        height={1}
        wrapMode="none"
        flexGrow={1}
        fg={RGBA.defaultForeground()}
        {...selecting(props.theme)}
      >
        {parts().path ?? ""}
        {parts().path && parts().branch ? (
          <span style={{ fg: props.theme.faint }}>{separator}</span>
        ) : (
          ""
        )}
        {parts().branch ?? ""}
      </text>
    </ShellRow>
  );
}
