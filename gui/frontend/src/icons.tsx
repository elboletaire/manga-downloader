// The handful of icons the window uses, inline so they ship with the bundle
// (no font, no network) and take the text colour. 24-unit grid, 1.75 stroke.

import type { ReactNode, SVGProps } from 'react';

type IconProps = SVGProps<SVGSVGElement> & { size?: number };

function icon(paths: ReactNode) {
  return function Icon({ size = 16, ...rest }: IconProps) {
    return (
      <svg
        width={size}
        height={size}
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth={1.75}
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden="true"
        focusable="false"
        {...rest}
      >
        {paths}
      </svg>
    );
  };
}

export const LinkIcon = icon(
  <>
    <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71" />
    <path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71" />
  </>,
);

export const DownloadIcon = icon(
  <>
    <path d="M12 4v11" />
    <path d="m7 10 5 5 5-5" />
    <path d="M5 20h14" />
  </>,
);

export const FolderIcon = icon(<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />);

export const CheckIcon = icon(<path d="m5 12.5 4.5 4.5L19 7.5" />);

export const XIcon = icon(
  <>
    <path d="M6 6l12 12" />
    <path d="M18 6 6 18" />
  </>,
);

export const AlertIcon = icon(
  <>
    <path d="M12 3.5 2.5 20h19z" />
    <path d="M12 10v4.5" />
    <path d="M12 17.5h.01" />
  </>,
);

export const ErrorIcon = icon(
  <>
    <circle cx="12" cy="12" r="9" />
    <path d="M12 7.5v5.5" />
    <path d="M12 16.5h.01" />
  </>,
);

export const RefreshIcon = icon(
  <>
    <path d="M20 12a8 8 0 1 1-2.34-5.66" />
    <path d="M20 4v5h-5" />
  </>,
);

export const SkipIcon = icon(
  <>
    <circle cx="12" cy="12" r="9" />
    <path d="M8 12h8" />
  </>,
);

export const PackageIcon = icon(
  <>
    <path d="M4 7.5 12 3l8 4.5v9L12 21l-8-4.5z" />
    <path d="M4 7.5 12 12l8-4.5" />
    <path d="M12 12v9" />
  </>,
);

export const LockIcon = icon(
  <>
    <rect x="5" y="11" width="14" height="9" rx="2" />
    <path d="M8 11V8a4 4 0 0 1 8 0v3" />
  </>,
);

export const LogIcon = icon(
  <>
    <path d="M4 6h16" />
    <path d="M4 12h16" />
    <path d="M4 18h10" />
  </>,
);

export const CopyIcon = icon(
  <>
    <rect x="8" y="8" width="12" height="12" rx="2" />
    <path d="M16 8V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h2" />
  </>,
);

export const SlidersIcon = icon(
  <>
    <path d="M4 6h10" />
    <path d="M18 6h2" />
    <circle cx="16" cy="6" r="2" />
    <path d="M4 12h2" />
    <path d="M10 12h10" />
    <circle cx="8" cy="12" r="2" />
    <path d="M4 18h12" />
    <path d="M20 18h0" />
    <circle cx="18" cy="18" r="2" />
  </>,
);

export const GearIcon = icon(
  <>
    <path d="M18.54 9.49L21 10.09L21 13.91L18.54 14.51L18.39 14.85L19.72 17.01L17.01 19.72L14.85 18.39L14.51 18.54L13.91 21L10.09 21L9.49 18.54L9.15 18.39L6.99 19.72L4.28 17.01L5.61 14.85L5.46 14.51L3 13.91L3 10.09L5.46 9.49L5.61 9.15L4.28 6.99L6.99 4.28L9.15 5.61L9.49 5.46L10.09 3L13.91 3L14.51 5.46L14.85 5.61L17.01 4.28L19.72 6.99L18.39 9.15Z" />
    <circle cx="12" cy="12" r="3" />
  </>,
);

export const ActivityIcon = icon(<path d="M3 12h4l3 7 4-14 3 7h4" />);

/** A spinner that inherits the text colour; it stops under reduced motion */
export function Spinner({ size = 14 }: { size?: number }) {
  return (
    <svg className="spinner" width={size} height={size} viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <circle cx="12" cy="12" r="9" fill="none" stroke="currentColor" strokeOpacity=".25" strokeWidth="3" />
      <path d="M21 12a9 9 0 0 0-9-9" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
    </svg>
  );
}

export const ChevronIcon = icon(<path d="m9 6 6 6-6 6" />);
