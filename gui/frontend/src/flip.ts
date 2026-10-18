// The glide from the simple view to the full one, done FLIP style: the shared
// elements are measured before the switch, then moved back to where they were
// and transitioned to their new place.

export interface Box {
  left: number;
  top: number;
  width: number;
  height: number;
}

/**
 * How an element is put back where it was: `scale` keeps its centre and its
 * proportions (the logo), `width` keeps its left edge and width, so text isn't
 * stretched (the URL form), and `move` keeps its centre only.
 */
export type FlipMode = 'scale' | 'width' | 'move';

/** The inline styles that put an element laid out at `last` back at `first` */
export function invert(first: Box, last: Box, mode: FlipMode): { transform: string; width?: string } {
  if (mode === 'width') {
    return {
      transform: `translate(${first.left - last.left}px, ${first.top - last.top}px)`,
      width: `${first.width}px`,
    };
  }
  const dx = first.left + first.width / 2 - (last.left + last.width / 2);
  const dy = first.top + first.height / 2 - (last.top + last.height / 2);
  if (mode === 'move' || last.height === 0) {
    return { transform: `translate(${dx}px, ${dy}px)` };
  }
  return { transform: `translate(${dx}px, ${dy}px) scale(${first.height / last.height})` };
}

export const FLIP_MS = 480;
const EASE = 'cubic-bezier(0.2, 0.7, 0.2, 1)';

/** Whether the user asked for less motion: the switch is instant then */
export function prefersReducedMotion(): boolean {
  return typeof window !== 'undefined' && window.matchMedia?.('(prefers-reduced-motion: reduce)').matches === true;
}

/** The boxes of the elements marked data-flip under root, by their mark */
export function measure(root: ParentNode): Map<string, Box> {
  const boxes = new Map<string, Box>();
  root.querySelectorAll<HTMLElement>('[data-flip]').forEach((el) => {
    boxes.set(el.dataset.flip!, el.getBoundingClientRect());
  });
  return boxes;
}

/** Moves each element still marked under root from its box in `first` to where it is now */
export function play(root: ParentNode, first: Map<string, Box>, modes: Record<string, FlipMode>): void {
  root.querySelectorAll<HTMLElement>('[data-flip]').forEach((el) => {
    const key = el.dataset.flip!;
    const from = first.get(key);
    if (from === undefined) return;
    const mode = modes[key] ?? 'move';
    const last = el.getBoundingClientRect();
    const to = invert(from, last, mode);
    el.style.transition = 'none';
    el.style.transform = to.transform;
    if (to.width !== undefined) {
      el.style.flex = 'none';
      el.style.width = to.width;
    }
    // the read commits the inverted styles, so the transition starts from them
    void el.offsetWidth;
    el.style.transition = `transform ${FLIP_MS}ms ${EASE}, width ${FLIP_MS}ms ${EASE}`;
    el.style.transform = '';
    if (to.width !== undefined) {
      el.style.width = `${last.width}px`;
    }
    setTimeout(() => {
      el.style.transition = el.style.transform = el.style.flex = el.style.width = '';
    }, FLIP_MS + 50);
  });
}
