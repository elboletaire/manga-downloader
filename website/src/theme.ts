// Theme choice: "system" follows the OS, "light"/"dark" force it. `forcedTheme`
// is inlined into every page's <head> through Function.prototype.toString()
// (see vite.config.ts), so keep it self-contained.

export const THEMES = ['system', 'light', 'dark'] as const
export type Theme = (typeof THEMES)[number]

/** The theme a radio value names, or null for anything else. */
export function parseTheme(value: string): Theme | null {
  return (THEMES as readonly string[]).includes(value) ? (value as Theme) : null
}

/** localStorage key holding the theme the visitor picked. */
export const THEME_KEY = 'md:theme'

/** The theme to force for a stored value, or null to follow the system. */
export function forcedTheme(stored: string | null): 'light' | 'dark' | null {
  return stored === 'light' || stored === 'dark' ? stored : null
}

/** Inline script that sets data-theme before first paint; works without storage. */
export function themeScript(): string {
  return (
    '<script>(function(){try{' +
    `var t=(${forcedTheme.toString()})(localStorage.getItem(${JSON.stringify(THEME_KEY)}));` +
    'if(t)document.documentElement.setAttribute("data-theme",t)' +
    '}catch(e){}})()</script>'
  )
}

/**
 * The media attribute for a source or meta written for `scheme`: the original
 * while following the system, otherwise matching only when it is the forced one.
 */
export function schemeMedia(original: string, scheme: 'light' | 'dark', forced: 'light' | 'dark' | null): string {
  if (!forced) return original
  return forced === scheme ? 'all' : 'not all'
}
