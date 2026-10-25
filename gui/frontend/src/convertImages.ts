// The window shows "convert images" as two checkboxes, but the setting is one
// comma-separated string (see grabber.ParseConvertFormats in Go). These map
// between the two, so a saved value of any of the four valid strings loads.

import type { T } from './i18n';

export interface ConvertFlags {
  avif: boolean;
  webp: boolean;
}

/** The checkboxes for a setting value. Unknown tokens are ignored. */
export function convertFlagsOf(value: string): ConvertFlags {
  const tokens = value
    .split(',')
    .map((t) => t.trim().toLowerCase())
    .filter((t) => t !== '');

  return { avif: tokens.includes('avif'), webp: tokens.includes('webp') };
}

/** The setting value for the checkboxes; neither checked is "none". */
export function convertValueOf(flags: ConvertFlags): string {
  if (flags.avif && flags.webp) {
    return 'avif,webp';
  }
  if (flags.avif) {
    return 'avif';
  }
  if (flags.webp) {
    return 'webp';
  }

  return 'none';
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
