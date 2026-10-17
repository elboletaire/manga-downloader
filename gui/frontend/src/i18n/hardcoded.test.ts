import { describe, expect, it } from 'vitest';

// The window's components and the helpers that word its state, as text: there's
// no @types/node to read the disk with. The mock and the tests are not read.
const sources = import.meta.glob(
  ['../*.tsx', '../{format,run,volumes,filters,browserVisible,convertImages,useWindow}.ts'],
  { query: '?raw', import: 'default', eager: true },
) as Record<string, string>;

// Exact text that may stay in English, each with the reason it isn't translated.
const ALLOWED: Record<string, string> = {
  'Manga Downloader': 'the app name, a brand',
  CBZ: 'an archive format, named as the CLI and the file names name it',
  EPUB: 'an archive format, named as the CLI and the file names name it',
  AVIF: 'an image format, named as the options name it',
  WebP: 'an image format, named as the options name it',
  JPEG: 'an image format, named as the options name it',
  'Series Vol. 01': 'a sample file name, as the filename template previews its output',
  'Series Vol. 01 [Group]': 'a sample file name, as the filename template previews its output',
  'Could not start the app: ${String(err)}':
    "main.tsx's fallback, shown only when the app can't start at all, before the catalog is there",
};

// Comment lines are not UI; the examples in them are not words the window shows.
const code = (text: string) =>
  text
    .split('\n')
    .filter((line) => !/^\s*(\/\/|\/\*|\*)/.test(line))
    .join('\n');

const WORD = /[A-Za-z]{2,}/;
// JSX text: the run after a tag's `>` (not an arrow's, nor a comparison's, which has a space before it)
// up to the next tag or expression; code in between, like `: boolean` or `&&`, is not text
const JSX_TEXT = /(?<=[\w"'}\]/])>([^<>{}\n;=()&|?]*)[<{]/g;
const ATTRIBUTE = /\b(aria-label|title|placeholder|alt|label|hint)="([^"]*)"/g;
// a quoted literal that opens with a capitalised word and a space, as in a sentence
const SENTENCE = /(['"`])([A-Z][a-z]+ [^'"`\n]*)\1/g;

const flagged: string[] = [];

for (const [path, raw] of Object.entries(sources)) {
  const text = code(raw);
  const file = path.replace(/^(\.\.\/|\.\/)+/, '');
  const report = (kind: string, found: string) => {
    const snippet = found.trim();
    if (WORD.test(snippet) && !Object.hasOwn(ALLOWED, snippet)) flagged.push(`${file} ${kind}: ${snippet}`);
  };
  for (const m of text.matchAll(JSX_TEXT)) report('text', m[1]);
  for (const m of text.matchAll(ATTRIBUTE)) report(m[1], m[2]);
  for (const m of text.matchAll(SENTENCE)) report('literal', m[2]);
}

describe('hardcoded strings', () => {
  it('finds no English text in the window that is not in the catalog', () => {
    expect(flagged).toEqual([]);
  });
});
