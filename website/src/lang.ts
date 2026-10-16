// Languages the site is published in, and the first-visit language redirect.
// `redirectTarget` is also inlined into the English page's <head> through
// Function.prototype.toString() (see vite.config.ts), so keep it
// self-contained: no imports, no closures, no inner functions.

export const LANGS = ['en', 'es', 'ca'] as const
export type Lang = (typeof LANGS)[number]

/** localStorage key holding the language the visitor picked in the switcher. */
export const LANG_KEY = 'md:lang'

export const LANG_INFO: Record<Lang, { name: string; ogLocale: string; path: string }> = {
  en: { name: 'English', ogLocale: 'en_US', path: '' },
  es: { name: 'Español', ogLocale: 'es_ES', path: 'es/' },
  ca: { name: 'Català', ogLocale: 'ca_ES', path: 'ca/' },
}

/**
 * Where the English page should send a first-time visitor: 'es' or 'ca' when
 * that is the first of their languages we publish, null otherwise. Never
 * redirects once the visitor has picked a language in the switcher.
 */
export function redirectTarget(languages: readonly string[], stored: string | null): 'es' | 'ca' | null {
  if (stored) return null
  for (let i = 0; i < languages.length; i++) {
    const primary = String(languages[i]).toLowerCase().split('-')[0]
    if (primary === 'en') return null
    if (primary === 'es' || primary === 'ca') return primary
  }
  return null
}

/** The link's href with `hash` as its fragment (any existing one replaced, dropped when `hash` is empty). */
export function withHash(href: string, hash: string): string {
  return href.split('#')[0] + (hash.length < 2 ? '' : hash)
}
