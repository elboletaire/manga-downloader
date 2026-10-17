import { describe, expect, it } from 'vitest';
import { LANGS } from './index';

type Tree = { [key: string]: string | Tree };

// The raw namespace files, keyed by "../locales/<lang>/<namespace>.json".
const files = import.meta.glob('../locales/*/*.json', { eager: true, import: 'default' }) as Record<string, Tree>;

const fileOf = (path: string) => /\/([^/]+)\/([^/]+)\.json$/.exec(path)!.slice(1);
const namespaces = (lang: string) =>
  Object.keys(files)
    .map(fileOf)
    .filter(([l]) => l === lang)
    .map(([, ns]) => ns)
    .sort();

function flatten(prefix: string, tree: Tree, out: Record<string, string> = {}): Record<string, string> {
  for (const [key, value] of Object.entries(tree)) {
    if (typeof value === 'string') out[`${prefix}.${key}`] = value;
    else flatten(`${prefix}.${key}`, value, out);
  }
  return out;
}

/** Every namespace of a language, flattened to `namespace.key`. */
function messages(lang: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [path, tree] of Object.entries(files)) {
    const [l, ns] = fileOf(path);
    if (l === lang) flatten(ns, tree, out);
  }
  return out;
}

const placeholders = (s: string) => [...s.matchAll(/\{\{\s*([\w.]+)\s*\}\}/g)].map((m) => m[1]).sort();
const tags = (s: string) => [...s.matchAll(/<\/?[a-z][a-z0-9]*/gi)].map((m) => m[0].toLowerCase()).sort();

const en = messages('en');

describe('locales', () => {
  for (const lang of LANGS.filter((l) => l !== 'en')) {
    const tr = messages(lang);

    it(`${lang} has the namespace files of en`, () => {
      expect(namespaces(lang)).toEqual(namespaces('en'));
    });

    it(`${lang} has exactly the keys of en`, () => {
      expect(Object.keys(tr).sort()).toEqual(Object.keys(en).sort());
    });

    it(`${lang} keeps every placeholder and tag, and fills every value`, () => {
      for (const [key, value] of Object.entries(en)) {
        expect(placeholders(tr[key] ?? ''), key).toEqual(placeholders(value));
        expect(tags(tr[key] ?? ''), key).toEqual(tags(value));
        expect((tr[key] ?? '').trim(), key).not.toBe('');
      }
    });

    it(`${lang} has an _other for every _one`, () => {
      for (const key of Object.keys(tr).filter((k) => k.endsWith('_one'))) {
        expect(tr[`${key.slice(0, -'_one'.length)}_other`], key).toBeTypeOf('string');
      }
    });
  }

  it('has an _other for every _one in en', () => {
    for (const key of Object.keys(en).filter((k) => k.endsWith('_one'))) {
      expect(en[`${key.slice(0, -'_one'.length)}_other`], key).toBeTypeOf('string');
    }
  });
});
