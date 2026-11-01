import { describe, expect, it } from "vitest";
import { shortcutFor, type KeyLike, type ShortcutContext } from "./shortcuts";

const key = (k: string, extra: Partial<KeyLike> = {}): KeyLike => ({
  key: k,
  ctrlKey: false,
  metaKey: false,
  altKey: false,
  target: { tagName: "BODY" },
  ...extra,
});
const open: ShortcutContext = { dialogOpen: false };

describe("shortcutFor", () => {
  it("maps keys to actions", () => {
    const table: [string, string][] = [
      ["j", "next"],
      ["ArrowDown", "next"],
      ["k", "prev"],
      ["ArrowUp", "prev"],
      ["x", "toggleSelect"],
      ["e", "reviewed"],
      ["a", "actioned"],
      ["i", "ignored"],
      ["n", "new"],
      ["Enter", "open"],
      ["Escape", "clearSelection"],
      ["/", "focusSearch"],
      ["?", "toggleHelp"],
    ];
    for (const [k, action] of table) expect(shortcutFor(key(k), open)).toBe(action);
  });

  it("returns null for other keys", () => {
    expect(shortcutFor(key("q"), open)).toBeNull();
    expect(shortcutFor(key("constructor"), open)).toBeNull();
  });

  it("is ignored in inputs, selects, textareas and editable content", () => {
    for (const tagName of ["INPUT", "SELECT", "TEXTAREA"]) {
      expect(shortcutFor(key("j", { target: { tagName } }), open)).toBeNull();
    }
    expect(shortcutFor(key("j", { target: { tagName: "DIV", isContentEditable: true } }), open)).toBeNull();
  });

  it("treats checkbox and radio inputs as not typing", () => {
    expect(shortcutFor(key("j", { target: { tagName: "INPUT", type: "checkbox" } }), open)).toBe("next");
    expect(shortcutFor(key("x", { target: { tagName: "INPUT", type: "radio" } }), open)).toBe("toggleSelect");
    expect(shortcutFor(key("j", { target: { tagName: "INPUT", type: "text" } }), open)).toBeNull();
    expect(shortcutFor(key("j", { target: { tagName: "INPUT", type: "search" } }), open)).toBeNull();
    expect(shortcutFor(key("j", { target: { tagName: "INPUT" } }), open)).toBeNull();
  });

  it("is ignored with modifiers", () => {
    expect(shortcutFor(key("j", { ctrlKey: true }), open)).toBeNull();
    expect(shortcutFor(key("j", { metaKey: true }), open)).toBeNull();
    expect(shortcutFor(key("j", { altKey: true }), open)).toBeNull();
  });

  it("is ignored while a dialog is open", () => {
    expect(shortcutFor(key("j"), { dialogOpen: true })).toBeNull();
  });

  it("leaves Enter to links and buttons", () => {
    expect(shortcutFor(key("Enter", { target: { tagName: "A" } }), open)).toBeNull();
    expect(shortcutFor(key("Enter", { target: { tagName: "BUTTON" } }), open)).toBeNull();
  });
});
