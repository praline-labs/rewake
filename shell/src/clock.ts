// Clock is time as the shell's life and its closing layer read it: the
// process's own, or a test's, which moves only when the test says so.
export type Clock = {
  now(): number;
  // after calls fn once ms have passed, unless the answer is called first.
  after(ms: number, fn: () => void): () => void;
};

export const realClock: Clock = {
  now: () => Date.now(),
  after(ms, fn) {
    const timer = setTimeout(fn, ms);
    return () => clearTimeout(timer);
  },
};
