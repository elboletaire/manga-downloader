// Keyboard movement in the chapter list: one focus stop for the whole list,
// with the arrows moving an "active" row, the way native list boxes work.
// Tabbing through thousands of checkboxes would be useless.

import type { Chapter } from './types';

/** The chapters that can be picked: the ones the listing marks locked can't */
export function pickable(chapters: Chapter[]): Chapter[] {
  return chapters.filter((c) => !c.locked);
}

/**
 * The row the key moves to, or null when the key doesn't move. page is how
 * many rows fit in the view.
 */
export function moveActive(key: string, active: number, count: number, page: number): number | null {
  if (count <= 0) {
    return null;
  }
  const step = Math.max(1, page - 1);
  const clamp = (i: number) => Math.min(count - 1, Math.max(0, i));
  // nothing active yet: any movement starts at the top
  const from = active < 0 ? -1 : active;

  switch (key) {
    case 'ArrowDown':
      return clamp(from + 1);
    case 'ArrowUp':
      return clamp(from < 0 ? 0 : from - 1);
    case 'PageDown':
      return clamp(from + step);
    case 'PageUp':
      return clamp(from - step);
    case 'Home':
      return 0;
    case 'End':
      return count - 1;
  }

  return null;
}

/** Shift with the vertical arrows extends the selection from the anchor */
export function extendsSpan(key: string, shift: boolean): boolean {
  return shift && (key === 'ArrowUp' || key === 'ArrowDown');
}

/** Every index from a to b inclusive, in either direction, in ascending order */
export function spanIndices(a: number, b: number): number[] {
  const lo = Math.min(a, b);
  const hi = Math.max(a, b);
  const out: number[] = [];
  for (let i = lo; i <= hi; i++) {
    out.push(i);
  }

  return out;
}
