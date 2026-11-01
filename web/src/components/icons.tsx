import type { ReactNode } from "react";

interface IconProps {
  className?: string;
}

function Svg({ className, children, filled }: IconProps & { children: ReactNode; filled?: boolean }) {
  const cls = className === undefined ? "icon" : `icon ${className}`;
  return (
    <svg
      className={cls}
      viewBox="0 0 16 16"
      width="16"
      height="16"
      fill={filled === true ? "currentColor" : "none"}
      stroke={filled === true ? "none" : "currentColor"}
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      {children}
    </svg>
  );
}

export const DotIcon = (p: IconProps) => (
  <Svg {...p} filled>
    <circle cx="8" cy="8" r="4.5" />
  </Svg>
);
export const EyeIcon = (p: IconProps) => (
  <Svg {...p}>
    <path d="M1.5 8s2.3-4.5 6.5-4.5S14.5 8 14.5 8s-2.3 4.5-6.5 4.5S1.5 8 1.5 8z" />
    <circle cx="8" cy="8" r="2" />
  </Svg>
);
export const CheckCircleIcon = (p: IconProps) => (
  <Svg {...p}>
    <circle cx="8" cy="8" r="6.5" />
    <path d="M5.5 8.2l1.8 1.8 3.2-3.6" />
  </Svg>
);
export const MinusCircleIcon = (p: IconProps) => (
  <Svg {...p}>
    <circle cx="8" cy="8" r="6.5" />
    <path d="M5.5 8h5" />
  </Svg>
);
export const CheckIcon = (p: IconProps) => (
  <Svg {...p}>
    <path d="M3.5 8.5l3 3 6-7" />
  </Svg>
);
export const XIcon = (p: IconProps) => (
  <Svg {...p}>
    <path d="M4 4l8 8M12 4l-8 8" />
  </Svg>
);
export const CopyIcon = (p: IconProps) => (
  <Svg {...p}>
    <rect x="5.5" y="5.5" width="8" height="8" rx="1.5" />
    <path d="M10.5 5.5v-2a1 1 0 0 0-1-1h-6a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h2" />
  </Svg>
);
export const SearchIcon = (p: IconProps) => (
  <Svg {...p}>
    <circle cx="7" cy="7" r="4.5" />
    <path d="M10.5 10.5L14 14" />
  </Svg>
);
export const ArrowLeftIcon = (p: IconProps) => (
  <Svg {...p}>
    <path d="M13 8H3M7 4L3 8l4 4" />
  </Svg>
);
export const ChevronDownIcon = (p: IconProps) => (
  <Svg {...p}>
    <path d="M4 6l4 4 4-4" />
  </Svg>
);
export const AlertCircleIcon = (p: IconProps) => (
  <Svg {...p}>
    <circle cx="8" cy="8" r="6.5" />
    <path d="M8 4.8v3.8M8 11.2v.1" />
  </Svg>
);
export const DownloadIcon = (p: IconProps) => (
  <Svg {...p}>
    <path d="M8 2.5v8M4.5 7.5L8 11l3.5-3.5M3 13.5h10" />
  </Svg>
);
