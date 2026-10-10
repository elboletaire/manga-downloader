import { describe, expect, it } from 'vitest';
import { scrollToReveal, visibleSpan } from './virtual';

describe('visibleSpan', () => {
  it('renders the rows in view plus the overscan', () => {
    // rows of 30, a 300 view scrolled to row 10: rows 10..20 visible
    expect(visibleSpan(300, 300, 30, 1000, 5)).toEqual({ first: 5, last: 25 });
  });

  it('clamps at both ends', () => {
    expect(visibleSpan(0, 300, 30, 1000, 5)).toEqual({ first: 0, last: 15 });
    expect(visibleSpan(29700, 300, 30, 1000, 5)).toEqual({ first: 985, last: 1000 });
    expect(visibleSpan(0, 300, 30, 4, 5)).toEqual({ first: 0, last: 4 });
  });

  it('is empty for an empty list', () => {
    expect(visibleSpan(0, 300, 30, 0, 5)).toEqual({ first: 0, last: 0 });
  });

  it('stays sane when scrolled past a list that shrank', () => {
    const span = visibleSpan(30000, 300, 30, 10, 5);
    expect(span.first).toBeLessThanOrEqual(span.last);
    expect(span.last).toBe(10);
  });
});

describe('scrollToReveal', () => {
  it('leaves a visible row alone', () => {
    expect(scrollToReveal(5, 30, 0, 300)).toBe(0);
  });

  it('scrolls up to a row above the view', () => {
    expect(scrollToReveal(2, 30, 300, 300)).toBe(60);
  });

  it('scrolls down just enough for a row below the view', () => {
    // row 12 ends at 390; a 300 view must start at 90
    expect(scrollToReveal(12, 30, 0, 300)).toBe(90);
  });
});
