import { expect, test } from "bun:test";
import { pathParts, tilded } from "./path.tsx";

test("a path under the home reads from ~, another as it is", () => {
  expect(tilded("/home/u/code/x", "/home/u")).toBe("~/code/x");
  expect(tilded("/home/u", "/home/u")).toBe("~");
  expect(tilded("/home/user2/x", "/home/u")).toBe("/home/user2/x");
  expect(tilded("/tmp/x", "")).toBe("/tmp/x");
});

// The owner, October 31, 2026: the top row is where the agent is now, cut
// from its start when long (`…/worktrees/wt-3836d0`), and its branch.
test("the top row cuts the path from its start and keeps the branch", () => {
  const state = { path: "/home/u/.rewake2-worktrees/rewake-44a66b/wt-3836d0", branch: "brief60" };
  const row = (width: number) => pathParts(state, width, "/home/u");
  expect(row(80)).toEqual({
    path: "~/.rewake2-worktrees/rewake-44a66b/wt-3836d0",
    branch: "⎇ brief60",
  });
  expect(row(40)).toEqual({ path: "…/rewake-44a66b/wt-3836d0", branch: "⎇ brief60" });
  expect(row(30)).toEqual({ path: "…/wt-3836d0", branch: "⎇ brief60" });
  expect(row(20)).toEqual({ path: "…-3836d0", branch: "⎇ brief60" });
  for (let width = 1; width <= 60; width++) {
    const { path = "", branch = "" } = row(width);
    const shown = path && branch ? `${path} · ${branch}` : path + branch;
    expect(Bun.stringWidth(shown)).toBeLessThanOrEqual(width);
  }
  // Too narrow for both, the branch stays, cut at its end.
  expect(row(12)).toEqual({ branch: "⎇ brief60" });
  expect(row(6)).toEqual({ branch: "⎇ bri…" });
  // Outside a repository the path alone; nothing before the path is known.
  expect(pathParts({ path: "/tmp/x" }, 4, "/home/u")).toEqual({ path: "…/x" });
  expect(pathParts({ branch: "main" }, 40)).toEqual({});
});
