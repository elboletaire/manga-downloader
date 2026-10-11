// Strings the scripts write at runtime. The page embeds its language's
// `runtime` section of locales/<lang>.json (see the site-data plugin), so the
// browser never loads i18next. Same {{name}} and _one/_other conventions.

export interface DownloadStrings {
  generic: string
  genericDetail: string
  /** Contains {{os}}. */
  forOS: string
  latest: string
  /** Contains {{tag}}. */
  desktopApp: string
}

export interface RuntimeStrings {
  download: DownloadStrings
  copy: { label: string; done: string }
  /** all_one/all_other ({{count}}), filtered_one/filtered_other ({{shown}}, {{count}}). */
  sites: Record<string, string>
}

/** English fallback; a test keeps it equal to locales/en.json's `runtime`. */
export const EN_RUNTIME: RuntimeStrings = {
  download: {
    generic: 'Download the desktop app',
    genericDetail: 'Pick your system in the downloads below',
    forOS: 'Download for {{os}}',
    latest: 'Latest release on GitHub',
    desktopApp: 'Desktop app {{tag}}',
  },
  copy: { label: 'Copy command', done: 'Copied' },
  sites: {
    all_one: '{{count}} site',
    all_other: '{{count}} sites',
    filtered_one: '{{shown}} of {{count}} site',
    filtered_other: '{{shown}} of {{count}} sites',
  },
}

/** Replaces {{name}} placeholders; unknown ones are left as they are. */
export function fill(template: string, vars: Record<string, string | number>): string {
  return template.replace(/\{\{\s*(\w+)\s*\}\}/g, (match, name: string) => (name in vars ? String(vars[name]) : match))
}

/** Picks `${key}_${plural form}` for `count` (falling back to `_other`) and fills it. */
export function plural(
  forms: Record<string, string>,
  key: string,
  count: number,
  lang: string,
  vars: Record<string, string | number> = {},
): string {
  const form = new Intl.PluralRules(lang).select(count)
  const template = forms[`${key}_${form}`] ?? forms[`${key}_other`] ?? ''
  return fill(template, { ...vars, count })
}
