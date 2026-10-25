// The desktop app's languages and translator. Catalogs are the JSON files in
// locales/<lang>/<namespace>.json, flattened to `namespace.key` keys.

export const LANGS = ['en', 'es', 'ca', 'ja'] as const;
export type Lang = (typeof LANGS)[number];

/** The saved preference: '' follows the system. */
export type LangPref = '' | Lang;

/** Each language's name in itself, never translated. */
export const LANG_INFO: Record<Lang, { name: string }> = {
  en: { name: 'English' },
  es: { name: 'Español' },
  ca: { name: 'Català' },
  ja: { name: '日本語' },
};

export type Vars = Record<string, string | number>;
export type T = (key: string, vars?: Vars) => string;
/** A message as written, its placeholders not filled yet. */
export type Template = (key: string, vars?: Vars) => string;
export type Catalog = Partial<Record<Lang, Record<string, string>>>;

const isLang = (s: string): s is Lang => (LANGS as readonly string[]).includes(s);

/** The saved language if any, else the first of the system's that we ship (by primary subtag), else English. */
export function resolveLanguage(pref: LangPref, languages: readonly string[]): Lang {
  if (isLang(pref)) return pref;
  for (const language of languages) {
    const primary = language.toLowerCase().split('-')[0];
    if (isLang(primary)) return primary;
  }
  return 'en';
}

/** Replaces {{name}} placeholders; unknown ones are left as they are. */
export function fill(template: string, vars: Vars = {}): string {
  return template.replace(/\{\{\s*(\w+)\s*\}\}/g, (match, name: string) =>
    Object.hasOwn(vars, name) ? String(vars[name]) : match,
  );
}

/**
 * The message for `lang` as written. With a `count`, `key_one` or `key_other` is
 * picked by the language's plural rules. A missing key falls back to English, then to the key.
 */
export function createTemplate(lang: Lang, catalog: Catalog = CATALOG): Template {
  const rules = new Intl.PluralRules(lang);
  return (key, vars = {}) => {
    const forms =
      vars.count === undefined ? [key] : [`${key}_${rules.select(Number(vars.count))}`, `${key}_other`, key];
    for (const table of [catalog[lang], catalog.en]) {
      for (const form of forms) {
        const text = table?.[form];
        if (text !== undefined) return text;
      }
    }
    return key;
  };
}

/** A translator for `lang`: the template with its placeholders filled. */
export function createT(lang: Lang, catalog: Catalog = CATALOG): T {
  const template = createTemplate(lang, catalog);
  return (key, vars = {}) => fill(template(key, vars), vars);
}

/**
 * A message the Go side sent: its key and args are translated, and its text is
 * the English rendering, shown when the catalog lacks the key. Without a
 * message, the plain text stands.
 */
export function say(
  t: T,
  msg: { key: string; args?: Record<string, unknown>; text: string } | undefined,
  text: string,
): string {
  if (!msg) return text;
  const out = t(msg.key, msg.args as Vars | undefined);
  return out === msg.key ? msg.text : out;
}

type Tree = { [key: string]: string | Tree };

/** Builds the catalogs from the namespace files, keyed by `<lang>/<namespace>.json`. */
export function loadCatalog(files: Record<string, unknown>): Catalog {
  const catalog: Record<string, Record<string, string>> = {};
  const flatten = (prefix: string, tree: Tree, out: Record<string, string>) => {
    for (const [key, value] of Object.entries(tree)) {
      if (typeof value === 'string') out[`${prefix}.${key}`] = value;
      else flatten(`${prefix}.${key}`, value, out);
    }
  };
  for (const [file, tree] of Object.entries(files)) {
    const [, lang, namespace] = /\/([^/]+)\/([^/]+)\.json$/.exec(file) ?? [];
    if (!isLang(lang)) continue;
    catalog[lang] ??= {};
    flatten(namespace, tree as Tree, catalog[lang]);
  }
  return catalog;
}

export const CATALOG: Catalog = loadCatalog(
  import.meta.glob('../locales/*/*.json', { eager: true, import: 'default' }),
);
