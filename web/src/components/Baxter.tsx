interface BaxterProps {
  size: number;
  /** Hide from assistive technology (the wordmark or heading is next to it). */
  decorative?: boolean;
  label?: string;
  className?: string;
}

function a11y(decorative: boolean | undefined, label: string | undefined) {
  return decorative === true
    ? ({ "aria-hidden": true, focusable: false } as const)
    : ({ role: "img", "aria-label": label ?? "FeedBaxter" } as const);
}

const EAR = "M96 62 C82 70 70 96 76 126 C80 146 92 150 96 136 C100 120 96 96 106 80 Z";
const HEAD = "M70 110 C70 78 90 54 118 54 C146 54 168 84 174 116 C178 140 156 162 128 168 C100 172 70 146 70 110 Z";
const SNOUT =
  "M163 106 C168 110 172 114 176 118 C184 130 178 142 166 142 C161 142 157 141 154 138 C150 128 153 116 163 106 Z";
const LINE =
  "M96 62 C82 70 70 96 76 126 C80 146 92 150 96 136 C100 120 96 96 106 80 C116 66 140 62 156 76 C168 88 170 108 176 118 C184 130 178 142 166 142 C156 142 150 150 142 160 C130 174 108 172 100 156";

const fill = (v: string) => ({ fill: `var(${v})` });
const stroke = (w: number) => ({
  stroke: "var(--ink)",
  strokeWidth: w,
  fill: "none",
  strokeLinecap: "round" as const,
  strokeLinejoin: "round" as const,
});

/** The full mark (64 px and up). */
export function BaxterFull({ size, decorative, label, className }: BaxterProps) {
  return (
    <svg
      className={className}
      viewBox="64 48 125 128"
      width={size}
      height={Math.round((size * 128) / 125)}
      {...a11y(decorative, label)}
    >
      <path d={HEAD} style={fill("--oat")} />
      <path transform="translate(1 0)" d={SNOUT} style={fill("--baxter-snout")} />
      <path transform="translate(5 4)" d={EAR} style={fill("--baxter-ear")} />
      <path d={LINE} style={stroke(2.2)} />
      <path d="M164 142 q-9 0 -14 -9" style={stroke(2.2)} />
      <ellipse cx="142" cy="100" rx="9.6" ry="10.8" style={fill("--ink")} />
      <circle cx="145" cy="96.4" r="2.5" style={fill("--card-stock")} />
      <circle cx="179" cy="121" r="6.8" style={fill("--dusk")} />
    </svg>
  );
}

/** The small mark (below 64 px). */
export function BaxterSmall({ size, decorative, label, className }: BaxterProps) {
  return (
    <svg className={className} viewBox="60 46 132 132" width={size} height={size} {...a11y(decorative, label)}>
      <path d={HEAD} style={fill("--oat")} />
      <path transform="translate(1 0)" d={SNOUT} style={fill("--baxter-snout")} />
      <path transform="translate(5 4)" d={EAR} style={fill("--baxter-ear")} />
      <path d={LINE} style={stroke(3.2)} />
      <path d="M164 142 q-9 0 -14 -9" style={stroke(3.2)} />
      <ellipse cx="142" cy="100" rx="10" ry="11.2" style={fill("--ink")} />
      <circle cx="179" cy="121" r="7.2" style={fill("--dusk")} />
    </svg>
  );
}
