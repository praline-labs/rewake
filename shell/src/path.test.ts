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
  // Too narrow for both, the branch goes and the path is cut further.
  expect(row(12)).toEqual({ path: "…/wt-3836d0" });
  expect(row(6)).toEqual({ path: "…836d0" });
  // Outside a repository the path alone; nothing before the path is known.
  expect(pathParts({ path: "/tmp/x" }, 4, "/home/u")).toEqual({ path: "…/x" });
  expect(pathParts({ branch: "main" }, 40)).toEqual({});
});

// The owner, October 31, 2026: the session's rewake name at the top row's
// right edge, for every pane; a narrow row cuts the path from its start
// first, then drops the branch, then cuts the name.
test("the top row ends with the session's name, cut last", () => {
  const state = {
    path: "/home/u/.rewake2-worktrees/rewake-44a66b/wt-3836d0",
    branch: "brief60",
    session: "review-pi",
  };
  const row = (width: number) => pathParts(state, width, "/home/u");
  expect(row(80)).toEqual({
    path: "~/.rewake2-worktrees/rewake-44a66b/wt-3836d0",
    branch: "⎇ brief60",
    name: "review-pi",
  });
  expect(row(40)).toEqual({ path: "…/wt-3836d0", branch: "⎇ brief60", name: "review-pi" });
  expect(row(30)).toEqual({ path: "…836d0", branch: "⎇ brief60", name: "review-pi" });
  expect(row(25)).toEqual({ path: "…/wt-3836d0", name: "review-pi" });
  expect(row(14)).toEqual({ path: "…0", name: "review-pi" });
  expect(row(13)).toEqual({ name: "review-pi" });
  expect(row(6)).toEqual({ name: "revie…" });
  for (let width = 1; width <= 80; width++) {
    const { path = "", branch = "", name = "" } = row(width);
    const left = path && branch ? `${path} · ${branch}` : path + branch;
    const gap = left && name ? 3 : 0;
    expect(Bun.stringWidth(left) + gap + Bun.stringWidth(name)).toBeLessThanOrEqual(width);
  }
  // Before the path is known the name stands alone.
  expect(pathParts({ session: "main-claude" }, 40)).toEqual({ name: "main-claude" });
});

// The owner, November 1, 2026: a worker's type moves from its strip to the
// top row, in brackets after its name; main has none. A narrow row loses the
// path first as before, then cuts the label from its end: the type goes
// before the name.
test("the top row names a worker with its type, cut before the name", () => {
  const state = { path: "/tmp/wt-1", session: "write-claude", type: "code" };
  const row = (width: number) => pathParts(state, width, "/home/u");
  expect(row(40)).toEqual({ path: "/tmp/wt-1", name: "write-claude (code)" });
  expect(row(20)).toEqual({ name: "write-claude (code)" });
  expect(row(17)).toEqual({ name: "write-claude (co…" });
  expect(row(13)).toEqual({ name: "write-claude…" });
  expect(row(12)).toEqual({ name: "write-claud…" });
  expect(pathParts({ path: "/tmp/x", session: "main-claude" }, 40, "/home/u")).toEqual({
    path: "/tmp/x",
    name: "main-claude",
  });
});
