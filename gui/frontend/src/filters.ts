// The language and scanlator selectors: what they offer once a series is
// loaded, and the labels for it. Pure, so the choices can be tested without a
// window; SourceBar decides when each one is shown.

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

/**
 * A filter value as the free-text field shows it. "all" and empty are the same
 * scanlator filter, so the field stays empty (placeholder) for both; a language
 * keeps "all", as empty there is the site's own default.
 */
export function typedValue(kind: FilterKind, value: string): string {
  return kind === 'scanlator' && value.toLowerCase() === FILTER_ALL ? '' : value;
}

/** Whether two values of one filter select the same thing */
export function sameFilter(kind: FilterKind, a: string, b: string): boolean {
  return typedValue(kind, a) === typedValue(kind, b);
}

/** The free-text placeholder: empty means no scanlator filter, or the site's default language */
export function filterPlaceholder(kind: FilterKind): string {
  return kind === 'scanlator' ? 'any' : 'site default';
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
 * The name of a language code for the selector: "es-la" reads as
 * "Spanish (Latin America)" when the browser knows it, and as the bare code
 * otherwise (an unknown language, or an invalid tag).
 */
export function languageName(code: string, locale = 'en'): string {
  try {
    const names = new Intl.DisplayNames([locale], { type: 'language' });
    const tag = LANGUAGE_TAGS[code.toLowerCase()] ?? code;
    const base = tag.split('-')[0];
    // an unknown language comes back as its own code, with the region in brackets
    if ((names.of(base) ?? base).toLowerCase() !== base.toLowerCase()) {
      return names.of(tag) ?? code;
    }
  } catch {
    // an invalid tag has no name: the code is the label
  }

  return code;
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
 * The options of one selector, in the order shown: "All" first where the site
 * takes it, then the values as the site lists them. The selected value is
 * always among them, even when the site doesn't list it (a value the series
 * was loaded with), so the select never shows the wrong one.
 */
export function filterOptions(filter: FilterChoice, kind: FilterKind, locale = 'en', loaded = true): FilterOption[] {
  const options: FilterOption[] = [];
  const selected = shownSelection(filter, loaded);
  if (selected === '') {
    // no value is shown: a disabled placeholder, so the select doesn't show the
    // first value as if it were the one the list is filtered by
    options.push({ value: '', label: kind === 'language' ? 'Choose a language…' : 'Choose a group…', disabled: true });
  }
  if (filter.all) {
    options.push({ value: FILTER_ALL, label: kind === 'language' ? 'All' : 'Any' });
  }

  const values = [...filter.values];
  if (selected !== '' && selected !== FILTER_ALL && !values.includes(selected)) {
    values.push(selected);
  }
  for (const value of values) {
    const label = kind === 'language' ? labelOfLanguage(value, locale) : value;
    options.push({ value, label });
  }

  return options;
}

function labelOfLanguage(code: string, locale: string): string {
  const name = languageName(code, locale);

  return name === code ? code : `${code} · ${name}`;
}
