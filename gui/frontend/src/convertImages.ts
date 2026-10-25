// The window shows "convert images" as checkboxes, but the setting is one
// comma-separated string (see grabber.ParseConvertFormats in Go). These map
// between the two, so a saved value of any of the valid strings loads.

import type { T } from './i18n';

export interface ConvertFlags {
  avif: boolean;
  webp: boolean;
  png: boolean;
  /** Every format but JPEG and GIF: the other boxes stay ticked and locked */
  all: boolean;
}

/** The checkboxes for a setting value. Unknown tokens are ignored. */
export function convertFlagsOf(value: string): ConvertFlags {
  const tokens = value
    .split(',')
    .map((t) => t.trim().toLowerCase())
    .filter((t) => t !== '');
  const all = tokens.includes('all');

  return {
    avif: all || tokens.includes('avif'),
    webp: all || tokens.includes('webp'),
    png: all || tokens.includes('png'),
    all,
  };
}

/** The setting value for the checkboxes; neither checked is "none". */
export function convertValueOf(flags: ConvertFlags): string {
  if (flags.all) {
    return 'all';
  }
  const formats = [flags.avif ? 'avif' : '', flags.webp ? 'webp' : '', flags.png ? 'png' : ''].filter((f) => f !== '');

  return formats.length > 0 ? formats.join(',') : 'none';
}

/** Shown under the AVIF box while the format is EPUB, which always converts it */
export function avifEpubHint(t: T): string {
  return t('options.pages.avifEpub');
}

export interface AvifSwitch {
  checked: boolean;
  disabled: boolean;
}

/**
 * The AVIF box for a format. EPUB always converts AVIF whatever the saved
 * setting says, so the box shows checked and locked then; the saved value
 * itself is left alone.
 */
export function avifSwitchOf(flags: ConvertFlags, format: string): AvifSwitch {
  if (format === 'epub') {
    return { checked: true, disabled: true };
  }
  return { checked: flags.avif, disabled: false };
}
