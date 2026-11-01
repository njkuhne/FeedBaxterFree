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
  return (
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
  );
}
