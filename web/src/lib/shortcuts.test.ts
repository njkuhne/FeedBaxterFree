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
const open: ShortcutContext = { dialogOpen: false, singleKeys: true };

describe("shortcutFor", () => {
  it("maps keys to actions", () => {
    const table: [string, string][] = [
      ["j", "next"],
      ["ArrowDown", "next"],
      ["k", "prev"],
      ["ArrowUp", "prev"],
      ["x", "toggleSelect"],
      ["X", "rangeSelect"],
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

  it("maps z to undo and u to back", () => {
    expect(shortcutFor(key("z"), open)).toBe("undo");
    expect(shortcutFor(key("u"), open)).toBe("back");
    expect(shortcutFor(key("z", { target: { tagName: "INPUT", type: "text" } }), open)).toBeNull();
    expect(shortcutFor(key("z", { metaKey: true }), open)).toBeNull();
    expect(shortcutFor(key("z"), { dialogOpen: true, singleKeys: true })).toBeNull();
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
    expect(shortcutFor(key("j"), { dialogOpen: true, singleKeys: true })).toBeNull();
  });

  it("leaves Enter to links and buttons", () => {
    expect(shortcutFor(key("Enter", { target: { tagName: "A" } }), open)).toBeNull();
    expect(shortcutFor(key("Enter", { target: { tagName: "BUTTON" } }), open)).toBeNull();
  });

  it("maps t to tag", () => {
    expect(shortcutFor(key("t"), open)).toBe("tag");
    expect(shortcutFor(key("t", { target: { tagName: "INPUT", type: "text" } }), open)).toBeNull();
  });

  it("r marks reviewed like e", () => {
    expect(shortcutFor(key("r"), open)).toBe("reviewed");
    expect(shortcutFor(key("e"), open)).toBe("reviewed");
  });

  it("Shift+j and Shift+k extend the selection", () => {
    expect(shortcutFor(key("J"), open)).toBe("extendNext");
    expect(shortcutFor(key("K"), open)).toBe("extendPrev");
    expect(shortcutFor(key("J", { target: { tagName: "INPUT", type: "text" } }), open)).toBeNull();
  });

  it("with single keys off, letters and slash do nothing", () => {
    const off: ShortcutContext = { dialogOpen: false, singleKeys: false };
    for (const k of ["j", "k", "x", "X", "J", "K", "e", "r", "a", "i", "n", "t", "u", "z", "/"]) {
      expect(shortcutFor(key(k), off)).toBeNull();
    }
  });

  it("with single keys off, arrows, Enter, Escape and ? still work", () => {
    const off: ShortcutContext = { dialogOpen: false, singleKeys: false };
    expect(shortcutFor(key("ArrowDown"), off)).toBe("next");
    expect(shortcutFor(key("ArrowUp"), off)).toBe("prev");
    expect(shortcutFor(key("Enter"), off)).toBe("open");
    expect(shortcutFor(key("Escape"), off)).toBe("clearSelection");
    expect(shortcutFor(key("?"), off)).toBe("toggleHelp");
  });
});
