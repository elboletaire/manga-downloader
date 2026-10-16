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
 * takes it, then the values in the backend's alphabetical order. The selected
 * value is always among them, even when the site doesn't list it (a value the
 * series was loaded with), so the select never shows the wrong one; it goes in
 * at its alphabetical place.
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
    options.push({ value: FILTER_ALL, label: kind === 'language' ? 'All' : 'All groups' });
  }

  const values = [...filter.values];
  if (selected !== '' && selected !== FILTER_ALL && !values.includes(selected)) {
    values.splice(insertionIndex(values, selected), 0, selected);
  }
  for (const value of values) {
    const label = kind === 'language' ? labelOfLanguage(value, locale) : value;
    options.push({ value, label });
  }

  return options;
}

// sortKey mirrors the backend's: lowercased, accents dropped
function sortKey(value: string): string {
  return value.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase();
}

function insertionIndex(sorted: string[], value: string): number {
  const compare = (a: string, b: string) => {
    const ka = sortKey(a);
    const kb = sortKey(b);
    // byte order like the backend, with digit runs as numbers
    const pa = ka.match(/\d+|\D+/g) ?? [];
    const pb = kb.match(/\d+|\D+/g) ?? [];
    for (let i = 0; i < Math.min(pa.length, pb.length); i++) {
      if (pa[i] === pb[i]) continue;
      if (/^\d/.test(pa[i]) && /^\d/.test(pb[i])) return Number(pa[i]) - Number(pb[i]);
      return pa[i] < pb[i] ? -1 : 1;
    }
    return pa.length - pb.length || (a < b ? -1 : a > b ? 1 : 0);
  };
  const at = sorted.findIndex((v) => compare(value, v) < 0);

  return at === -1 ? sorted.length : at;
}

function labelOfLanguage(code: string, locale: string): string {
  const name = languageName(code, locale);

  return name === code ? code : `${code} · ${name}`;
}
