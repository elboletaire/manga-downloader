import { describe, expect, it } from 'vitest';
import { code } from './lintSource';

// The window's components and the helpers that word its state, as text: there's
// no @types/node to read the disk with. The mock and the tests are not read.
const sources = import.meta.glob(
  ['../*.tsx', '../{format,run,volumes,filters,browserVisible,convertImages,useWindow}.ts'],
  { query: '?raw', import: 'default', eager: true },
) as Record<string, string>;
const HELPERS = /\/(format|run|volumes|filters|browserVisible|convertImages|useWindow)\.ts$/;

// Exact text that may stay in English, each with the reason it isn't translated.
const ALLOWED: Record<string, string> = {
  'Manga Downloader': 'the app name, a brand',
  CBZ: 'an archive format, named as the CLI and the file names name it',
  EPUB: 'an archive format, named as the CLI and the file names name it',
  AVIF: 'an image format, named as the options name it',
  WebP: 'an image format, named as the options name it',
  JPEG: 'an image format, named as the options name it',
  'Series Vol. 01 (1-7)': 'a sample file name, as the filename template previews its output',
  'Series Vol. 01 (1-7) [Group]': 'a sample file name, as the filename template previews its output',
  'Could not start the app: ${String(err)}':
    "main.tsx's fallback, shown only when the app can't start at all, before the catalog is there",
};

// Values a helper returns for the code to compare, not text the window shows.
const RETURNED_VALUES: Record<string, string> = {
  danger: 'a tone name, the class that colours a row',
  neutral: 'a tone name, the class that colours a row',
  success: 'a tone name, the class that colours a row',
  text: 'a filter kind, the control the code renders',
  select: 'a filter kind, the control the code renders',
  'avif,webp': 'a convert-images setting, the value the options file stores',
  avif: 'a convert-images setting, the value the options file stores',
  webp: 'a convert-images setting, the value the options file stores',
  none: 'a convert-images setting, the value the options file stores',
};

const WORD = /[A-Za-z]{2,}/;
// JSX text: the run after a tag's `>` up to the next tag or expression. A `>` with a
// space before it is a comparison and `=>` an arrow, so neither starts text.
const JSX_TEXT = /(?<=[\w"'}\]/])>([^<>{}\n]*)[<{]/g;
// Text reads as words when two words sit side by side, or it is one word with a stop;
// code such as `('options')` or `: boolean` has neither.
const READABLE = /[A-Za-z]+\s+[A-Za-z]|^[A-Za-z]{2,}[.!?…]*$/;
const ATTRIBUTE = /\b(aria-label|title|placeholder|alt|label|hint)="([^"]*)"/g;
// a quoted literal that opens with a capitalised word and a space, as in a sentence
const SENTENCE = /(['"`])([A-Z][a-z]+ [^'"`\n]*)\1/g;
// a quoted literal a helper returns (template literals are not checked)
const RETURNED = /\breturn\s+(['"])([^'"\\\n]*)\1/g;

const flagged: string[] = [];

for (const [path, raw] of Object.entries(sources)) {
  const text = code(raw);
  const file = path.replace(/^(\.\.\/|\.\/)+/, '');
  const report = (kind: string, found: string) => {
    const snippet = found.trim();
    if (WORD.test(snippet) && !Object.hasOwn(ALLOWED, snippet)) flagged.push(`${file} ${kind}: ${snippet}`);
  };
  for (const m of text.matchAll(JSX_TEXT)) {
    const snippet = m[1].trim();
    if (READABLE.test(snippet) && !Object.hasOwn(ALLOWED, snippet)) flagged.push(`${file} text: ${snippet}`);
  }
  for (const m of text.matchAll(ATTRIBUTE)) report(m[1], m[2]);
  for (const m of text.matchAll(SENTENCE)) report('literal', m[2]);
  if (HELPERS.test(path)) {
    for (const m of text.matchAll(RETURNED)) {
      const value = m[2];
      if (/[A-Za-z]/.test(value) && !Object.hasOwn(ALLOWED, value) && !Object.hasOwn(RETURNED_VALUES, value)) {
        flagged.push(`${file} return: ${value}`);
      }
    }
  }
}

describe('hardcoded strings', () => {
  it('finds no English text in the window that is not in the catalog', () => {
    expect(flagged).toEqual([]);
  });
});
