import { describe, expect, it } from 'vitest';
import { avifEpubHint, avifSwitchOf, convertFlagsOf, convertPdfHint, convertValueOf, webpSwitchOf } from './convertImages';
import { createT } from './i18n';

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

describe('avifSwitchOf', () => {
  it('follows the saved value for the other formats', () => {
    expect(avifSwitchOf({ avif: true, webp: false }, 'cbz')).toEqual({ checked: true, disabled: false });
    expect(avifSwitchOf({ avif: false, webp: true }, 'raw')).toEqual({ checked: false, disabled: false });
  });

  it('shows AVIF checked and locked for EPUB and PDF, whatever the saved value', () => {
    expect(avifSwitchOf({ avif: false, webp: false }, 'epub')).toEqual({ checked: true, disabled: true });
    expect(avifSwitchOf({ avif: false, webp: true }, 'epub')).toEqual({ checked: true, disabled: true });
    expect(avifSwitchOf({ avif: false, webp: false }, 'pdf')).toEqual({ checked: true, disabled: true });
  });
});

describe('webpSwitchOf', () => {
  it('follows the saved value for non-PDF formats and locks on for PDF', () => {
    expect(webpSwitchOf({ avif: true, webp: false }, 'cbz')).toEqual({ checked: false, disabled: false });
    expect(webpSwitchOf({ avif: false, webp: true }, 'epub')).toEqual({ checked: true, disabled: false });
    expect(webpSwitchOf({ avif: false, webp: false }, 'pdf')).toEqual({ checked: true, disabled: true });
  });
});

describe('avifEpubHint and convertPdfHint', () => {
  it('translates the EPUB and PDF notes', () => {
    expect(avifEpubHint(createT('en'))).toBe("Always on for EPUB: readers can't show AVIF.");
    expect(avifEpubHint(createT('ca'))).toBe('Sempre activat per a EPUB: els lectors no mostren AVIF.');
    expect(convertPdfHint(createT('en'))).toBe("Always on for PDF: PDFs can't embed AVIF or WebP.");
    expect(convertPdfHint(createT('ca'))).toBe('Sempre activat per a PDF: els PDF no admeten AVIF ni WebP.');
  });
});
