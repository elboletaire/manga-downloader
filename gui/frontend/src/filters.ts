// The language and scanlator selectors: what they offer once a series is
// loaded, and the labels for it. Pure, so the choices can be tested without a
// window; SourceBar decides when each one is shown.

import type { Lang, T } from './i18n';
import type { FilterChoice, State } from './types';

export type FilterKind = 'language' | 'scanlator';

export interface FilterOption {
  value: string;
  label: string;
  disabled?: boolean;
}

/** The sentinel the backend takes for "every value" (grabber.FilterAll) */
export const FILTER_ALL = 'all';

/**
 * Whether a load has been attempted. Before that the two filters are free
 * text; after, each one is a selector of the series' own values, or disabled
 * when the site lists none.
 */
export function filtersIdentified(state: Pick<State, 'loaded' | 'loadError'>): boolean {
  return state.loaded || state.loadError !== '';
}

// Codes the sites use that aren't BCP 47: "es-la" is Latin American Spanish,
// which BCP 47 writes "es-419" (read as is, "la" is the region of Laos)
const LANGUAGE_TAGS: Record<string, string> = { 'es-la': 'es-419' };

/** A stored "all" shows as the empty field, whose placeholder says "any"; the stored value stays */
export function typedValue(value: string): string {
  return value.toLowerCase() === FILTER_ALL ? '' : value;
}

/** Whether two scanlator values select the same thing: "" and "all" both keep every group */
export function sameScanlator(a: string, b: string): boolean {
  return typedValue(a) === typedValue(b);
}

/** How a filter is shown: free text, a selector of the series' values, or disabled */
export type FilterControl = 'text' | 'select' | 'disabled';

/**
 * How a filter is shown. Free text until a load, and also after a load that
 * couldn't read the site (unknown site, network down): its values are unknown,
 * so the typed value stays editable to retry. A selector when the site lists
 * values; disabled only when the loaded site lists none.
 */
export function filterControl(state: Pick<State, 'loaded' | 'loadError'>, choice: FilterChoice | null): FilterControl {
  if (!filtersIdentified(state)) {
    return 'text';
  }
  if (choice !== null) {
    return 'select';
  }

  return state.loaded ? 'disabled' : 'text';
}

/**
 * The name of a language code for the selector, in the UI language: "es-la"
 * reads as "Spanish (Latin America)" when the browser knows it, and as the bare
 * code otherwise (an unknown language, or an invalid tag).
 */
export function languageName(code: string, lang: Lang = 'en'): string {
  try {
    const names = new Intl.DisplayNames([lang], { type: 'language' });
    const tag = LANGUAGE_TAGS[code.toLowerCase()] ?? code;
    const base = tag.split('-')[0];
    // an unknown language comes back as its own code, with the region in brackets
    if ((names.of(base) ?? base).toLowerCase() !== base.toLowerCase()) {
      return capitalise(names.of(tag) ?? code, lang);
    }
  } catch {
    // an invalid tag has no name: the code is the label
  }

  return code;
}

// es and ca names come back lowercase ("inglés"), where a label starts with a capital
function capitalise(name: string, lang: Lang): string {
  return name.charAt(0).toLocaleUpperCase(lang) + name.slice(1);
}

/**
 * The value a selector shows as picked. After a failed load the selected value
 * is only what was typed, so one the site doesn't list is not shown as if it
 * were an option: the placeholder stands in.
 */
export function shownSelection(filter: FilterChoice, loaded: boolean): string {
  if (!loaded && filter.selected !== FILTER_ALL && !filter.values.includes(filter.selected)) {
    return '';
  }

  return filter.selected;
}

/**
 * The options of one selector, in the order shown: a placeholder when no value
 * is shown, "All" first where the site takes it, then the values sorted with
 * the UI language's collation (digit runs as numbers). The selected value is
 * always among them, even when the site doesn't list it (a value the series
 * was loaded with), so the select never shows the wrong one.
 */
export function filterOptions(filter: FilterChoice, kind: FilterKind, t: T, lang: Lang = 'en', loaded = true): FilterOption[] {
  const options: FilterOption[] = [];
  const selected = shownSelection(filter, loaded);
  if (selected === '') {
    // no value is shown: a disabled placeholder, so the select doesn't show the
    // first value as if it were the one the list is filtered by
    options.push({ value: '', label: kind === 'language' ? t('filters.chooseLanguage') : t('filters.chooseGroup'), disabled: true });
  }
  if (filter.all) {
    options.push({ value: FILTER_ALL, label: kind === 'language' ? t('common.all') : t('filters.allGroups') });
  }

  const values = [...filter.values];
  if (selected !== '' && selected !== FILTER_ALL && !values.includes(selected)) {
    values.push(selected);
  }
  const collator = new Intl.Collator(lang, { numeric: true });
  values.sort(collator.compare);
  for (const value of values) {
    const label = kind === 'language' ? labelOfLanguage(value, lang) : value;
    options.push({ value, label });
  }

  return options;
}

function labelOfLanguage(code: string, lang: Lang): string {
  const name = languageName(code, lang);

  return name === code ? code : `${code} · ${name}`;
}
