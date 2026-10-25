// Languages the site is published in, and the language a visitor prefers
// (`preferredLang`, which main.ts uses to offer their page in a banner).

export const LANGS = ['en', 'es', 'ca'] as const
export type Lang = (typeof LANGS)[number]

/** localStorage key holding the language the visitor picked in the switcher (or closed the banner on). */
export const LANG_KEY = 'md:lang'

export const LANG_INFO: Record<Lang, { name: string; ogLocale: string; path: string }> = {
  en: { name: 'English', ogLocale: 'en_US', path: '' },
  es: { name: 'Español', ogLocale: 'es_ES', path: 'es/' },
  ca: { name: 'Català', ogLocale: 'ca_ES', path: 'ca/' },
}

/**
 * The first of the visitor's languages we publish, or null when there is none
 * or they have already picked one. Never offers anything once `stored` is set.
 */
export function preferredLang(languages: readonly string[], stored: string | null): Lang | null {
  if (stored) return null
  for (const language of languages) {
    const primary = String(language).toLowerCase().split('-')[0]
    if ((LANGS as readonly string[]).includes(primary)) return primary as Lang
  }
  return null
}

/** The link's href with `hash` as its fragment (any existing one replaced, dropped when `hash` is empty). */
export function withHash(href: string, hash: string): string {
  return href.split('#')[0] + (hash.length < 2 ? '' : hash)
}
