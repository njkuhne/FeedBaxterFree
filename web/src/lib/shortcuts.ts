export type ShortcutAction =
  | "next"
  | "prev"
  | "extendNext"
  | "extendPrev"
  | "toggleSelect"
  | "rangeSelect"
  | "open"
  | "reviewed"
  | "actioned"
  | "ignored"
  | "new"
  | "clearSelection"
  | "focusSearch"
  | "toggleHelp"
  | "undo"
  | "tag"
  | "back";

export interface KeyLike {
  key: string;
  ctrlKey: boolean;
  metaKey: boolean;
  altKey: boolean;
  target: { tagName?: string; isContentEditable?: boolean; type?: string | undefined } | null;
}

export interface ShortcutContext {
  dialogOpen: boolean;
  /** false: every single-character shortcut except "?" is off (WCAG 2.1.4). Read it from getPrefs() at key time. */
  singleKeys: boolean;
}

const TYPING = new Set(["INPUT", "SELECT", "TEXTAREA"]);
/** Inputs that take no typed text: row checkboxes keep the list shortcuts working. */
const NON_TEXT_INPUT = new Set(["checkbox", "radio"]);
const NATIVE_ENTER = new Set(["A", "BUTTON", "SUMMARY"]);

const MAP: Record<string, ShortcutAction> = {
  j: "next",
  ArrowDown: "next",
  k: "prev",
  J: "extendNext",
  K: "extendPrev",
  ArrowUp: "prev",
  x: "toggleSelect",
  X: "rangeSelect",
  Enter: "open",
  e: "reviewed",
  r: "reviewed",
  a: "actioned",
  i: "ignored",
  n: "new",
  Escape: "clearSelection",
  "/": "focusSearch",
  "?": "toggleHelp",
  z: "undo",
  t: "tag",
  u: "back",
};

/** Map a key event to a list action. Null when the key is ignored in this context. */
export function shortcutFor(e: KeyLike, ctx: ShortcutContext): ShortcutAction | null {
  if (ctx.dialogOpen || e.ctrlKey || e.metaKey || e.altKey) return null;
  const tag = e.target?.tagName?.toUpperCase();
  const nonText = tag === "INPUT" && NON_TEXT_INPUT.has((e.target?.type ?? "").toLowerCase());
  if (e.target?.isContentEditable === true || (tag !== undefined && TYPING.has(tag) && !nonText)) return null;
  const action = Object.prototype.hasOwnProperty.call(MAP, e.key) ? MAP[e.key] : undefined;
  if (action === undefined) return null;
  if (!ctx.singleKeys && e.key.length === 1 && e.key !== "?") return null;
  if (action === "open" && tag !== undefined && NATIVE_ENTER.has(tag)) return null;
  return action;
}
