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

// the timing is styles.css's --glide and --ease, so the CSS and the glide stay in step
const GLIDE = 'var(--glide) var(--ease)';

/** The glide's length, read from --glide (ms) */
export function glideMs(): number {
  const ms = parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--glide'));
  return Number.isNaN(ms) ? 0 : ms;
}

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

// each element's pending reset of its inline styles, so a new glide isn't cut short by the last one's
const resets = new WeakMap<HTMLElement, ReturnType<typeof setTimeout>>();

/** Moves each element still marked under root from its box in `first` to where it is now */
export function play(root: ParentNode, first: Map<string, Box>, modes: Record<string, FlipMode>): void {
  const ms = glideMs();
  root.querySelectorAll<HTMLElement>('[data-flip]').forEach((el) => {
    const key = el.dataset.flip!;
    const from = first.get(key);
    if (from === undefined) return;
    const mode = modes[key] ?? 'move';
    const last = el.getBoundingClientRect();
    // hidden in the full view (the wordmark on narrow windows): nothing to glide to
    if (last.width === 0) return;
    const to = invert(from, last, mode);
    el.style.transition = 'none';
    el.style.transform = to.transform;
    if (to.width !== undefined) {
      el.style.flex = 'none';
      el.style.width = to.width;
    }
    // the read commits the inverted styles, so the transition starts from them
    void el.offsetWidth;
    el.style.transition = `transform ${GLIDE}, width ${GLIDE}`;
    el.style.transform = '';
    if (to.width !== undefined) {
      el.style.width = `${last.width}px`;
    }
    clearTimeout(resets.get(el));
    resets.set(
      el,
      setTimeout(() => {
        el.style.transition = el.style.transform = el.style.flex = el.style.width = '';
      }, ms + 50),
    );
  });
}

/** Ends a glide under root at once: the pending resets are dropped and the inline styles removed */
export function stop(root: ParentNode): void {
  root.querySelectorAll<HTMLElement>('[data-flip]').forEach((el) => {
    clearTimeout(resets.get(el));
    el.style.transition = el.style.transform = el.style.flex = el.style.width = '';
  });
}
