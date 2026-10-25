import { describe, expect, it } from 'vitest'
import { preferredLang, withHash } from './lang'

describe('preferredLang', () => {
  it.each([
    [['es-ES', 'en'], null, 'es'],
    [['ca'], null, 'ca'],
    [['ca-ES', 'es-ES'], null, 'ca'],
    [['es-ES', 'ca-ES'], null, 'es'],
    [['fr-FR', 'ca'], null, 'ca'],
    [['ES-mx'], null, 'es'],
    [['ca-ES-valencia'], null, 'ca'],
    [['en-US', 'es'], null, 'en'],
    [['fr-FR', 'en-GB'], null, 'en'],
    [['de', 'fr'], null, null],
    [[], null, null],
    [['ja-JP', 'en'], null, 'ja'],
    [['en-US', 'ja'], null, 'en'],
    [['ja'], 'en', null],
    [['es'], 'en', null],
    [['ca'], 'es', null],
  ] as const)('%j with stored %s gives %s', (languages, stored, want) => {
    expect(preferredLang(languages, stored)).toBe(want)
  })
})

describe('withHash', () => {
  it.each([
    ['/es/', '#faq-old', '/es/#faq-old'],
    ['/es/#sites', '#faq-old', '/es/#faq-old'],
    ['/es/#sites', '', '/es/'],
    ['/', '#', '/'],
  ])('%s with %j gives %s', (href, hash, want) => {
    expect(withHash(href, hash)).toBe(want)
  })
})
