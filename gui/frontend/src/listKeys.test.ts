import { describe, expect, it } from 'vitest';
import { extendsSpan, moveActive, pickable, spanIndices } from './listKeys';
import type { Chapter } from './types';

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

describe('extendsSpan', () => {
  it('is true for Shift with the vertical arrows only', () => {
    expect(extendsSpan('ArrowDown', true)).toBe(true);
    expect(extendsSpan('ArrowUp', true)).toBe(true);
    expect(extendsSpan('ArrowDown', false)).toBe(false);
    expect(extendsSpan('PageDown', true)).toBe(false);
    expect(extendsSpan(' ', true)).toBe(false);
  });
});

describe('pickable', () => {
  it('leaves out the locked chapters, keeping the order', () => {
    const ch = (index: number, locked: boolean): Chapter => ({
      index,
      number: index,
      title: '',
      language: '',
      group: '',
      volume: 0,
      locked,
    });
    expect(pickable([ch(0, false), ch(1, true), ch(2, false)]).map((c) => c.index)).toEqual([0, 2]);
    expect(pickable([ch(0, true)])).toEqual([]);
  });
});
