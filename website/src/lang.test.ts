import { describe, expect, it } from 'vitest'
import { redirectTarget, withHash } from './lang'

describe('redirectTarget', () => {
  it.each([
    [['es-ES', 'en'], null, 'es'],
    [['ca'], null, 'ca'],
    [['ca-ES', 'es-ES'], null, 'ca'],
    [['es-ES', 'ca-ES'], null, 'es'],
    [['fr-FR', 'ca'], null, 'ca'],
    [['ES-mx'], null, 'es'],
    [['ca-ES-valencia'], null, 'ca'],
    [['en-US', 'es'], null, null],
    [['en-GB', 'ca'], null, null],
    [['de', 'fr'], null, null],
    [[], null, null],
    [['es'], 'en', null],
    [['ca'], 'es', null],
  ] as const)('%j with stored %s gives %s', (languages, stored, want) => {
    expect(redirectTarget(languages, stored)).toBe(want)
  })

  it('works standalone, as inlined into the page', () => {
    const inlined = new Function(`return (${redirectTarget.toString()})`)() as typeof redirectTarget
    expect(inlined(['ca', 'es'], null)).toBe('ca')
    expect(inlined(['es'], 'en')).toBe(null)
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
