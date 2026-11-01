import { useEffect, useRef } from "react";
import { VisuallyHidden } from "./VisuallyHidden";

export interface SegmentedOption<V extends string> {
  value: V;
  label: string;
  count?: number;
}

interface SegmentedProps<V extends string> {
  name: string;
  label: string;
  options: SegmentedOption<V>[];
  value: V;
  onChange: (value: V) => void;
}

export function Segmented<V extends string>({ name, label, options, value, onChange }: SegmentedProps<V>) {
  const scroller = useRef<HTMLDivElement | null>(null);

  // Below 880 px the options scroll sideways; keep the checked one fully visible. Horizontal only:
  // scrollIntoView could also move the page. Without layout (jsdom) the widths are 0 and nothing happens.
  useEffect(() => {
    const box = scroller.current;
    const el = box?.querySelector<HTMLElement>("input:checked")?.closest<HTMLElement>("label");
    if (!box || !el || box.clientWidth === 0) return;
    const left = el.offsetLeft;
    const right = left + el.offsetWidth;
    if (left < box.scrollLeft) box.scrollLeft = left;
    else if (right > box.scrollLeft + box.clientWidth) box.scrollLeft = right - box.clientWidth;
  }, [value]);

  return (
    <div className="segmented-scroll" ref={scroller}>
      <fieldset className="segmented" role="radiogroup">
        <legend>
          <VisuallyHidden>{label}</VisuallyHidden>
        </legend>
        {options.map((o) => (
          <label key={o.value} className="segmented-option">
            <input
              type="radio"
              name={name}
              value={o.value}
              checked={o.value === value}
              onChange={() => onChange(o.value)}
            />
            <span>{o.count === undefined ? o.label : `${o.label} ${o.count}`}</span>
          </label>
        ))}
      </fieldset>
    </div>
  );
}
