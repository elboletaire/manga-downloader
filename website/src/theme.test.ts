import { describe, expect, it } from 'vitest'
import { forcedTheme, parseTheme, schemeMedia, themeScript, THEME_KEY } from './theme'

describe('forcedTheme', () => {
  it.each([
    ['light', 'light'],
    ['dark', 'dark'],
    ['system', null],
    ['', null],
    [null, null],
    ['Dark', null],
  ])('%s -> %s', (stored, want) => {
    expect(forcedTheme(stored)).toBe(want)
  })
})

describe('parseTheme', () => {
  it('accepts the three themes only', () => {
    expect(parseTheme('system')).toBe('system')
    expect(parseTheme('dark')).toBe('dark')
    expect(parseTheme('sepia')).toBeNull()
  })
})

describe('schemeMedia', () => {
  const dark = '(prefers-color-scheme: dark)'
  it('keeps the original media while following the system', () => {
    expect(schemeMedia(dark, 'dark', null)).toBe(dark)
  })
  it('matches only the forced scheme', () => {
    expect(schemeMedia(dark, 'dark', 'dark')).toBe('all')
    expect(schemeMedia(dark, 'dark', 'light')).toBe('not all')
    expect(schemeMedia('(prefers-color-scheme: light)', 'light', 'dark')).toBe('not all')
  })
})

describe('themeScript', () => {
  const run = (value: string | null, broken = false) => {
    const attrs: Record<string, string> = {}
    const localStorage = {
      getItem: (k: string) => {
        if (broken) throw new Error('denied')
        expect(k).toBe(THEME_KEY)
        return value
      },
    }
    const document = { documentElement: { setAttribute: (k: string, v: string) => (attrs[k] = v) } }
    const js = themeScript().replace(/^<script>|<\/script>$/g, '')
    new Function('localStorage', 'document', js)(localStorage, document)
    return attrs
  }

  it('sets data-theme for a stored choice', () => {
    expect(run('dark')).toEqual({ 'data-theme': 'dark' })
    expect(run('light')).toEqual({ 'data-theme': 'light' })
  })
  it('leaves the system theme alone, with or without storage', () => {
    expect(run('system')).toEqual({})
    expect(run(null)).toEqual({})
    expect(run(null, true)).toEqual({})
  })
})
