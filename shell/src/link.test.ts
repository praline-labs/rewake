import { expect, test } from "bun:test";
import { readLine } from "./link.ts";

// The refusal's shape is the one thing every version of the host keeps: a
// shell of another version says why it was turned away and stops.
test("a host's refusal stops the shell with the host's words", () => {
  const read = readLine(`{"refused":"this host speaks 1, the client 2","speaks":[1]}`);
  expect(read).toEqual({
    stop: "the host refused this shell: this host speaks 1, the client 2 (it speaks 1)",
  });
});

test("a message reads, and a line that does not stops the shell", () => {
  expect(readLine(`{"t":"main","main":{"pid":7,"socket":"/s"}}`)).toEqual({
    message: { t: "main", main: { pid: 7, socket: "/s" } },
  });
  expect("stop" in readLine("not json")).toBe(true);
  expect("stop" in readLine(`{"x":1}`)).toBe(true);
});
