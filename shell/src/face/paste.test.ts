import { expect, test } from "bun:test";
import { draftOf, keptFaces, restoreFaces, setDraft } from "./faces.ts";
import { pastesOf } from "./paste.ts";

// A draft a restart hands on keeps what its paste markers stand for: the
// next shell sends the paste, not the marker (docs/shell.md, "Restart,
// crash and close"). The handing on is JSON, as over the host's channel.
test("a restart hands a draft's pastes on with it", () => {
  const original = "one\ntwo\nthree\nfour";
  const paste = pastesOf("worker").take(original);
  setDraft("worker", `look: ${paste.text}`);
  const kept = JSON.parse(JSON.stringify(keptFaces()));
  // The next shell starts with nothing of the old one's.
  restoreFaces({ modes: [], drafts: [] });
  expect(pastesOf("worker").markers(`look: ${paste.text}`)).toEqual([]);
  restoreFaces(kept);
  const draft = draftOf("worker");
  const live = new Set(
    pastesOf("worker")
      .markers(draft)
      .map((m) => m.id),
  );
  expect(pastesOf("worker").expand(draft, live)).toBe(`look: ${original}`);
  // A paste after the restart takes the next number, not one in use.
  expect(pastesOf("worker").take("a\nb\nc\nd").id).toBe(2);
});
