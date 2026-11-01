import { describe, expect, it } from "vitest";
import { bytes, duration, plural } from "./format";

describe("format", () => {
  it("formats bytes in binary units with one decimal", () => {
    expect(bytes(0)).toBe("0 B");
    expect(bytes(1023)).toBe("1023 B");
    expect(bytes(1024)).toBe("1.0 KB");
    expect(bytes(1048576)).toBe("1.0 MB");
    expect(bytes(1572864)).toBe("1.5 MB");
    expect(bytes(5 * 1024 ** 3)).toBe("5.0 GB");
  });

  it("formats durations", () => {
    expect(duration(0)).toBe("0 s");
    expect(duration(45)).toBe("45 s");
    expect(duration(125)).toBe("2 min 5 s");
    expect(duration(120)).toBe("2 min");
    expect(duration(3780)).toBe("1 h 3 min");
    expect(duration(7200)).toBe("2 h");
  });

  it("pluralises", () => {
    expect(plural(1, "item", "items")).toBe("1 item");
    expect(plural(0, "item", "items")).toBe("0 items");
    expect(plural(12, "item", "items")).toBe("12 items");
  });
});
