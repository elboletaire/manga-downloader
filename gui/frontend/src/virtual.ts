// Windowing for long lists: only the rows in view (plus a margin) are
// rendered, so a series with thousands of chapters scrolls as fast as one
// with ten. Rows have a fixed height, which keeps the arithmetic trivial.

import { useCallback, useLayoutEffect, useRef, useState } from 'react';

export interface Span {
  /** First rendered row, inclusive */
  first: number;
  /** Last rendered row, exclusive */
  last: number;
}

/** The rows to render for a scroll position, with overscan rows either side */
export function visibleSpan(
  scrollTop: number,
  viewHeight: number,
  rowHeight: number,
  count: number,
  overscan: number,
): Span {
  if (count <= 0 || rowHeight <= 0) {
    return { first: 0, last: 0 };
  }
  const first = Math.max(0, Math.floor(scrollTop / rowHeight) - overscan);
  const last = Math.min(count, Math.ceil((scrollTop + Math.max(0, viewHeight)) / rowHeight) + overscan);

  return { first: Math.min(first, last), last };
}

/**
 * The scroll position that brings a row fully into view, moving as little as
 * possible: unchanged when the row is already visible.
 */
export function scrollToReveal(index: number, rowHeight: number, scrollTop: number, viewHeight: number): number {
  const top = index * rowHeight;
  const bottom = top + rowHeight;
  if (top < scrollTop) {
    return top;
  }
  if (bottom > scrollTop + viewHeight) {
    return Math.max(0, bottom - viewHeight);
  }

  return scrollTop;
}

/**
 * Tracks a scroller's position and height. The returned ref goes on the
 * scrolling element; its height follows the layout through a ResizeObserver,
 * so the list can simply fill whatever room it's given.
 */
export function useVirtualList(count: number, rowHeight: number, overscan = 8) {
  const ref = useRef<HTMLDivElement>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const [viewHeight, setViewHeight] = useState(600);

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) {
      return;
    }
    setViewHeight(el.clientHeight);
    if (typeof ResizeObserver === 'undefined') {
      return;
    }
    const observer = new ResizeObserver(() => setViewHeight(el.clientHeight));
    observer.observe(el);

    return () => observer.disconnect();
  }, []);

  const onScroll = useCallback((e: React.UIEvent<HTMLElement>) => setScrollTop(e.currentTarget.scrollTop), []);

  const reveal = useCallback(
    (index: number) => {
      const el = ref.current;
      if (!el) {
        return;
      }
      const next = scrollToReveal(index, rowHeight, el.scrollTop, el.clientHeight);
      if (next !== el.scrollTop) {
        el.scrollTop = next;
        setScrollTop(next);
      }
    },
    [rowHeight],
  );

  const reset = useCallback(() => {
    setScrollTop(0);
    if (ref.current) {
      ref.current.scrollTop = 0;
    }
  }, []);

  const span = visibleSpan(scrollTop, viewHeight, rowHeight, count, overscan);

  return { ref, onScroll, span, viewHeight, reveal, reset };
}
