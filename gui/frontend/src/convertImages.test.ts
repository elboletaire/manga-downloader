import { describe, expect, it } from 'vitest';
import { avifEpubHint, avifSwitchOf, convertFlagsOf, convertValueOf, type ConvertFlags } from './convertImages';
import { createT } from './i18n';

describe('convertFlagsOf and convertValueOf', () => {
  it('maps the valid settings to the checkboxes and back', () => {
    const none = { avif: false, webp: false, png: false, all: false };
    const cases: Array<[string, ConvertFlags]> = [
      ['avif,webp,png', { avif: true, webp: true, png: true, all: false }],
      ['avif,webp', { ...none, avif: true, webp: true }],
      ['avif', { ...none, avif: true }],
      ['webp', { ...none, webp: true }],
      ['png', { ...none, png: true }],
      ['avif,png', { ...none, avif: true, png: true }],
      ['none', none],
      // all ticks the three format boxes too, so unticking it keeps them
      ['all', { avif: true, webp: true, png: true, all: true }],
    ];
    for (const [value, flags] of cases) {
      expect(convertFlagsOf(value)).toEqual(flags);
      expect(convertValueOf(flags)).toBe(value);
    }
  });

  it('loads saved values that differ in order, case or spacing', () => {
    expect(convertFlagsOf('webp, AVIF')).toMatchObject({ avif: true, webp: true });
    expect(convertValueOf(convertFlagsOf('webp, AVIF'))).toBe('avif,webp');
    expect(convertValueOf(convertFlagsOf('PNG, Webp'))).toBe('webp,png');
  });

  it('treats an empty or unknown value as nothing to convert', () => {
    const none = { avif: false, webp: false, png: false, all: false };
    expect(convertFlagsOf('')).toEqual(none);
    expect(convertFlagsOf('jpeg')).toEqual(none);
    expect(convertValueOf(none)).toBe('none');
  });

  it('gives all over the format boxes when it is ticked', () => {
    expect(convertValueOf({ avif: false, webp: false, png: true, all: true })).toBe('all');
  });
});

describe('avifSwitchOf', () => {
  it('follows the saved value for the other formats', () => {
    expect(avifSwitchOf({ avif: true, webp: false, png: false, all: false }, 'cbz')).toEqual({ checked: true, disabled: false });
    expect(avifSwitchOf({ avif: false, webp: true, png: false, all: false }, 'raw')).toEqual({ checked: false, disabled: false });
  });

  it('shows AVIF checked and locked for EPUB, whatever the saved value', () => {
    expect(avifSwitchOf({ avif: false, webp: false, png: false, all: false }, 'epub')).toEqual({ checked: true, disabled: true });
    expect(avifSwitchOf({ avif: false, webp: true, png: false, all: false }, 'epub')).toEqual({ checked: true, disabled: true });
  });
});

describe('avifEpubHint', () => {
  it('translates the EPUB note', () => {
    expect(avifEpubHint(createT('en'))).toBe("Always on for EPUB: readers can't show AVIF.");
    expect(avifEpubHint(createT('ca'))).toBe('Sempre activat per a EPUB: els lectors no mostren AVIF.');
  });
});
