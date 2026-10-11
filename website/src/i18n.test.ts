import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { EN_RUNTIME, fill, plural } from './i18n'
import { LANGS } from './lang'

type Tree = { [key: string]: string | Tree }

const read = (rel: string) => readFileSync(fileURLToPath(new URL(rel, import.meta.url)), 'utf8')
const locale = (lang: string) => JSON.parse(read(`../locales/${lang}.json`)) as Tree

function flatten(tree: Tree, prefix = ''): Record<string, string> {
  const out: Record<string, string> = {}
  for (const [k, v] of Object.entries(tree)) {
    const key = prefix ? `${prefix}.${k}` : k
    if (typeof v === 'string') out[key] = v
    else Object.assign(out, flatten(v, key))
  }
  return out
}

const placeholders = (s: string) => [...s.matchAll(/\{\{\s*([\w.]+)\s*\}\}/g)].map((m) => m[1]).sort()
const tags = (s: string) => [...s.matchAll(/<\/?[a-z][a-z0-9]*/gi)].map((m) => m[0].toLowerCase()).sort()

const en = flatten(locale('en'))

describe('locales', () => {
  for (const lang of LANGS.filter((l) => l !== 'en')) {
    const tr = flatten(locale(lang))

    it(`${lang} has exactly the keys of en`, () => {
      expect(Object.keys(tr).sort()).toEqual(Object.keys(en).sort())
    })

    it(`${lang} keeps every placeholder and tag`, () => {
      for (const [key, value] of Object.entries(en)) {
        expect(placeholders(tr[key] ?? ''), key).toEqual(placeholders(value))
        expect(tags(tr[key] ?? ''), key).toEqual(tags(value))
        expect((tr[key] ?? '').trim(), key).not.toBe('')
      }
    })
  }

  it('has every key the page uses', () => {
    for (const [, , key, count] of read('../index.html').matchAll(/\{\{(t|a|j):([\w.]+)(?::(\w+))?\}\}/g)) {
      if (count) {
        expect(en[`${key}_one`], key).toBeTypeOf('string')
        expect(en[`${key}_other`], key).toBeTypeOf('string')
      } else {
        expect(en[key], key).toBeTypeOf('string')
      }
    }
  })

  it('bundles the same English runtime strings as en.json', () => {
    expect(EN_RUNTIME).toEqual(locale('en').runtime)
  })
})

describe('fill and plural', () => {
  it('fills known placeholders and keeps unknown ones', () => {
    expect(fill('Download for {{os}}', { os: 'Linux' })).toBe('Download for Linux')
    expect(fill('{{a}} {{b}}', { a: 1 })).toBe('1 {{b}}')
  })

  it('picks the plural form and falls back to _other', () => {
    const forms = EN_RUNTIME.sites
    expect(plural(forms, 'all', 1, 'en')).toBe('1 site')
    expect(plural(forms, 'all', 120, 'en')).toBe('120 sites')
    expect(plural(forms, 'filtered', 120, 'en', { shown: 3 })).toBe('3 of 120 sites')
    // Spanish has a "many" form for millions; it falls back to _other.
    expect(plural(forms, 'all', 1000000, 'es')).toBe('1000000 sites')
  })
})
