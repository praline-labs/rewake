import { expect, test } from "bun:test";
import { outerTitle } from "./title.ts";

test("only main's title goes to the outer terminal", () => {
  expect(outerTitle(true, "✳ rewake")).toBe("✳ rewake");
  expect(outerTitle(false, "π - rewake")).toBeUndefined();
  expect(outerTitle(true, undefined)).toBeUndefined();
  expect(outerTitle(true, "")).toBeUndefined();
});
