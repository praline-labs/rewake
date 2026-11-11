import { expect, test } from "bun:test";
import { contract } from "../../face/src/messages.ts";
import { hintsFor } from "./hints.ts";
import { version } from "./link.ts";
import { bannerOf, contracts, helloContracts } from "./update.ts";

test("the banner says what the room's plan asks of the person", () => {
  const cases: [Parameters<typeof bannerOf>[0], string | undefined][] = [
    [undefined, undefined],
    [[], undefined],
    [[{ part: "shell", class: "shell", why: "changed" }], "Ctrl+\\ r loads the updated shell"],
    [[{ part: "rewake", class: "room", why: "changed" }], "room restart needed: rewake"],
    [
      [
        { part: "shell", class: "shell", why: "changed" },
        { part: "rewake", class: "switch", why: "changed" },
      ],
      "rewake restart rewake switches the room to the updated rewake",
    ],
    [
      [
        { part: "shell", class: "shell", why: "changed" },
        { part: "rewake", class: "room", why: "changed" },
      ],
      "room restart needed: rewake",
    ],
    // A part that reloads itself, or that new sessions take, asks nothing.
    [[{ part: "face", class: "live", why: "changed" }], undefined],
    [[{ part: "pi", class: "sessions", why: "changed" }], undefined],
  ];
  for (const [steps, want] of cases) {
    const hint = bannerOf(steps);
    expect(hint ? `${hint.keys} ${hint.does}` : undefined).toBe(want);
  }
});

test("the banner shows a newer release on npm until the plan asks for more", () => {
  const available = { version: "2.13.0", restarts: "shell" } as const;
  const hint = bannerOf([], available);
  expect(hint ? `${hint.keys} ${hint.does}` : undefined).toBe(
    "2.13.0 available · restarts: shell · rewake update",
  );
  const nothing = bannerOf(undefined, { version: "2.13.0", restarts: "nothing" });
  expect(nothing ? `${nothing.keys} ${nothing.does}` : undefined).toBe(
    "2.13.0 available · restarts: nothing · rewake update",
  );
  // Installed, the plan's own step is what the person must do now.
  const installed = bannerOf([{ part: "shell", class: "shell", why: "changed" }], available);
  expect(installed ? `${installed.keys} ${installed.does}` : undefined).toBe(
    "Ctrl+\\ r loads the updated shell",
  );
});

test("the banner stands right after where the keys go, never in the prefix's list", () => {
  const update = { keys: "Ctrl+\\ r", does: "loads the updated shell" };
  const base = { focus: "main", name: "main", prefix: false, zoomed: false, workers: 0 } as const;
  const hints = hintsFor({ ...base, update });
  expect(hints[0]?.keys).toBe("keys");
  expect(hints[1]).toEqual(update);
  expect(hintsFor({ ...base, prefix: true, update })).not.toContainEqual(update);
});

test("the shell says the contracts it loaded", () => {
  expect(contracts()).toEqual({
    shell: { requires: { host: String(version), face: String(contract), "room-view": "3" } },
    face: { speaks: { face: String(contract) }, requires: { mirror: "3" } },
  });
});

// Found in review: a compiled face moves only with its shell, so the hello
// carries its contracts, and the host judges a restart of the shell by them.
test("the hello carries the contracts of a face compiled into the shell", () => {
  const own = { host: String(version), face: String(contract), "room-view": "3" };
  expect(helloContracts(true)).toEqual({
    requires: own,
    face: { speaks: { face: String(contract) }, requires: { mirror: "3" } },
  });
  expect(helloContracts(false)).toEqual({ requires: own });
});
