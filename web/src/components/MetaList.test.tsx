import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { MetaItem, MetaList } from "./MetaList";

describe("MetaList", () => {
  it("meta list renders items with no separator text nodes", () => {
    const { container } = render(
      <MetaList className="x">
        <MetaItem>Reviewed</MetaItem>
        <MetaItem>usr_1</MetaItem>
        <MetaItem>iOS</MetaItem>
      </MetaList>,
    );
    const list = container.querySelector("ul.meta-list.x");
    expect(list?.querySelectorAll("li")).toHaveLength(3);
    const walker = document.createTreeWalker(container, NodeFilter.SHOW_TEXT);
    const texts: string[] = [];
    for (let n = walker.nextNode(); n !== null; n = walker.nextNode()) texts.push(n.textContent ?? "");
    expect(texts).toEqual(["Reviewed", "usr_1", "iOS"]);
    for (const t of texts) expect(/^\s*·\s*$/.test(t)).toBe(false);
  });
});
