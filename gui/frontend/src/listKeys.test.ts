import { describe, expect, it } from 'vitest';
import { moveActive, spanIndices } from './listKeys';

describe('moveActive', () => {
  it('steps with the arrows, clamped to the list', () => {
    expect(moveActive('ArrowDown', 3, 10, 5)).toBe(4);
    expect(moveActive('ArrowDown', 9, 10, 5)).toBe(9);
    expect(moveActive('ArrowUp', 0, 10, 5)).toBe(0);
    expect(moveActive('ArrowUp', 3, 10, 5)).toBe(2);
  });

  it('starts at the top when nothing is active', () => {
    expect(moveActive('ArrowDown', -1, 10, 5)).toBe(0);
    expect(moveActive('ArrowUp', -1, 10, 5)).toBe(0);
  });

  it('pages by one less than a view, and jumps to the ends', () => {
    expect(moveActive('PageDown', 0, 100, 10)).toBe(9);
    expect(moveActive('PageUp', 50, 100, 10)).toBe(41);
    expect(moveActive('PageDown', 95, 100, 10)).toBe(99);
    expect(moveActive('Home', 50, 100, 10)).toBe(0);
    expect(moveActive('End', 5, 100, 10)).toBe(99);
  });

  it('ignores other keys and empty lists', () => {
    expect(moveActive('a', 3, 10, 5)).toBeNull();
    expect(moveActive('ArrowDown', 0, 0, 5)).toBeNull();
  });
});

describe('spanIndices', () => {
  it('covers both ends in either direction', () => {
    expect(spanIndices(2, 5)).toEqual([2, 3, 4, 5]);
    expect(spanIndices(5, 2)).toEqual([2, 3, 4, 5]);
    expect(spanIndices(4, 4)).toEqual([4]);
  });
});
