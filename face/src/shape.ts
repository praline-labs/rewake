// The words a message's shape is written in (messages.ts): each field of a
// message the face posts is said once, as a value, and both its type and the
// shell's hold on it (shell/src/face/check.ts) come from that value, so a
// field is added in one place and the two cannot drift.

// Bound names a limit the shell sets on a text or a list; the numbers are
// the shell's own (check.ts).
export type Bound = "text" | "name" | "copy" | "rows" | "spans";

// Field is what one field may hold: a text within a bound, a flag, a count
// (a small whole number), a serial (any whole number from 0), one of a list
// of words, or a list of objects of a shape. optional lets it be absent.
export type Field = (
  | { is: "text"; bound: Bound }
  | { is: "flag" }
  | { is: "count" }
  | { is: "serial" }
  | { is: "one"; of: readonly string[] }
  | { is: "list"; of: Shape; bound: Bound; item: string }
) & { optional?: true };

// Shape is an object's fields by name.
export type Shape = { readonly [key: string]: Field };

export const text = (bound: Bound) => ({ is: "text", bound }) as const;
export const flag = { is: "flag" } as const;
export const count = { is: "count" } as const;
export const serial = { is: "serial" } as const;
export const one = <const V extends readonly string[]>(of: V) => ({ is: "one", of }) as const;
// list is a list of objects of a shape; item names one of them in a fault.
export const list = <const S extends Shape>(of: S, bound: Bound, item: string) =>
  ({ is: "list", of, bound, item }) as const;
export const optional = <const F extends Field>(field: F) =>
  ({ ...field, optional: true }) as const;

type Value<F> = F extends { is: "text" }
  ? string
  : F extends { is: "flag" }
    ? boolean
    : F extends { is: "count" | "serial" }
      ? number
      : F extends { is: "one"; of: readonly (infer V)[] }
        ? V
        : F extends { is: "list"; of: infer S }
          ? Of<S>[]
          : never;

type Optional<S> = { [K in keyof S]: S[K] extends { optional: true } ? K : never }[keyof S];
type Flat<T> = { [K in keyof T]: T[K] };

// Of is the type of an object of a shape.
export type Of<S> = Flat<
  { -readonly [K in Exclude<keyof S, Optional<S>>]: Value<S[K]> } & {
    -readonly [K in Optional<S>]?: Value<S[K]>;
  }
>;
