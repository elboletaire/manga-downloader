import { describe, expect, it } from 'vitest';
import { convertFlagsOf, convertValueOf } from './convertImages';

describe('convertFlagsOf and convertValueOf', () => {
  it('maps the four valid settings to the checkboxes and back', () => {
    const cases: Array<[string, { avif: boolean; webp: boolean }]> = [
      ['avif,webp', { avif: true, webp: true }],
      ['avif', { avif: true, webp: false }],
      ['webp', { avif: false, webp: true }],
      ['none', { avif: false, webp: false }],
    ];
    for (const [value, flags] of cases) {
      expect(convertFlagsOf(value)).toEqual(flags);
      expect(convertValueOf(flags)).toBe(value);
    }
  });

  it('loads saved values that differ in order, case or spacing', () => {
    expect(convertFlagsOf('webp, AVIF')).toEqual({ avif: true, webp: true });
    expect(convertValueOf(convertFlagsOf('webp, AVIF'))).toBe('avif,webp');
  });

  it('treats an empty or unknown value as nothing to convert', () => {
    expect(convertFlagsOf('')).toEqual({ avif: false, webp: false });
    expect(convertFlagsOf('jpeg')).toEqual({ avif: false, webp: false });
    expect(convertValueOf({ avif: false, webp: false })).toBe('none');
  });
});
