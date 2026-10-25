// What the permission hooks are told of a call and a dialog, and how a call
// is named: the shapes of the engine's events the asks read (asks.ts).

export interface CheckEvent {
  tool: string;
  input: unknown;
  tool_use_id?: string;
  ceiling?: unknown;
  // isReadOnly is the tool's word that the call only reads (2.1.304+),
  // absent when it does not say.
  isReadOnly?: true;
}

export interface QuestionEvent {
  tool_use_id?: string;
  questions: ReadonlyArray<{
    question: string;
    header?: string;
    multiSelect?: boolean;
    options: ReadonlyArray<{ label: string; description?: string } | string>;
  }>;
}

// DialogEvent is the engine's own permission dialog, as
// classic.PermissionRequest tells it when the dialog is drawn: no call id.
export interface DialogEvent {
  tool_name: string;
  tool_input: unknown;
}

// DialogDecision answers such a dialog, as PermissionRequest's decision.
export type DialogDecision = { behavior: "allow" } | { behavior: "deny"; message: string };

// callKey names a call by its tool and input, as both the check and the
// dialog that follows it see them.
export const callKey = (tool: string, input: unknown) => tool + "\u0000" + canonical(input);

// canonical is JSON with every object's keys in order, so one call's input
// gives one key however its fields were written.
export function canonical(value: unknown): string {
  if (Array.isArray(value)) return "[" + value.map(canonical).join(",") + "]";
  if (value !== null && typeof value === "object") {
    const object = value as Record<string, unknown>;
    return (
      "{" +
      Object.keys(object)
        .sort()
        .map((key) => JSON.stringify(key) + ":" + canonical(object[key]))
        .join(",") +
      "}"
    );
  }
  return JSON.stringify(value) ?? "null";
}

// summary is what the permission question names of the call.
export function summary(tool: string, input: unknown): string {
  const fields = (input ?? {}) as Record<string, unknown>;
  const named = fields.command ?? fields.file_path ?? fields.path ?? fields.url ?? fields.pattern;
  const text = typeof named === "string" ? named : canonical(input);
  return text.length > 120 ? text.slice(0, 117) + "..." : text;
}
