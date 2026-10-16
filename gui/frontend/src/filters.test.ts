import { describe, expect, it } from 'vitest';
import { sameScanlator, typedValue, filterControl, filtersIdentified, filterOptions, languageName, shownSelection } from './filters';
import type { FilterChoice } from './types';

const choice = (over: Partial<FilterChoice>): FilterChoice => ({
  values: [],
  all: false,
  selected: '',
  ...over,
});

describe('filtersIdentified', () => {
  it('is false before any load was attempted', () => {
    expect(filtersIdentified({ loaded: false, loadError: '' })).toBe(false);
  });

  it('is true once a series loaded or a load failed', () => {
    expect(filtersIdentified({ loaded: true, loadError: '' })).toBe(true);
    expect(filtersIdentified({ loaded: false, loadError: 'site not recognised' })).toBe(true);
  });
});

describe('languageName', () => {
  it('names es-la as Latin American Spanish, not the region Laos', () => {
    expect(languageName('es-la', 'en')).toBe('Latin American Spanish');
  });

  it('falls back to the code when the name would only repeat it', () => {
    expect(languageName('zz-qq', 'en')).toBe('zz-qq');
  });

  it('falls back to the code for an invalid tag', () => {
    expect(languageName('not a tag!', 'en')).toBe('not a tag!');
  });
});

describe('filterOptions', () => {
  it('puts All first, labelled for the kind', () => {
    const langs = filterOptions(choice({ values: ['en'], all: true, selected: 'all' }), 'language');
    expect(langs[0]).toEqual({ value: 'all', label: 'All' });

    const groups = filterOptions(choice({ values: ['Alpha'], all: true, selected: 'all' }), 'scanlator');
    expect(groups[0]).toEqual({ value: 'all', label: 'All groups' });
  });

  it('keeps the backend order and slots in an unlisted selection alphabetically', () => {
    const options = filterOptions(choice({ values: ['Dawn', 'Écarlate', 'Fable', 'Zed'], selected: 'Ecru' }), 'scanlator');
    expect(options.map((o) => o.value)).toEqual(['Dawn', 'Écarlate', 'Ecru', 'Fable', 'Zed']);

    const accented = filterOptions(choice({ values: ['Dawn', 'Fable', 'Zed'], selected: 'Écarlate' }), 'scanlator');
    expect(accented.map((o) => o.value)).toEqual(['Dawn', 'Écarlate', 'Fable', 'Zed']);
  });

  it('has no All option where the site has none', () => {
    const options = filterOptions(choice({ values: ['en', 'es'], selected: 'en' }), 'language');
    expect(options.map((o) => o.value)).toEqual(['en', 'es']);
  });

  it('labels languages with the code and their name, groups by name', () => {
    const langs = filterOptions(choice({ values: ['es-la'], selected: 'es-la' }), 'language');
    expect(langs[0].label).toBe('es-la · Latin American Spanish');

    const groups = filterOptions(choice({ values: ['Alpha'], selected: 'Alpha' }), 'scanlator');
    expect(groups[0].label).toBe('Alpha');
  });

  it('keeps the selected value even when the site does not list it', () => {
    const options = filterOptions(choice({ values: ['en'], selected: 'fr' }), 'language');
    expect(options.map((o) => o.value)).toEqual(['en', 'fr']);
  });

  it('does not offer the typed value of a failed load', () => {
    const failed = choice({ values: ['en'], selected: 'ca' });
    expect(shownSelection(failed, false)).toBe('');
    const options = filterOptions(failed, 'language', 'en', false);
    expect(options.map((o) => o.value)).toEqual(['', 'en']);
    expect(options[0].disabled).toBe(true);
    expect(shownSelection(choice({ values: ['en'], selected: 'en' }), false)).toBe('en');
  });

  it('does not repeat a listed selected value', () => {
    const options = filterOptions(choice({ values: ['en', 'es'], selected: 'es' }), 'language');
    expect(options.map((o) => o.value)).toEqual(['en', 'es']);
  });

  it('leads with a disabled placeholder when the site shows no single value', () => {
    const groups = filterOptions(choice({ values: ['Alpha', 'Delta'], selected: '' }), 'scanlator');
    expect(groups[0]).toEqual({ value: '', label: 'Choose a group…', disabled: true });
    expect(groups.slice(1).map((o) => o.value)).toEqual(['Alpha', 'Delta']);

    const languages = filterOptions(choice({ values: ['en', 'es'], selected: '' }), 'language');
    expect(languages[0]).toEqual({ value: '', label: 'Choose a language…', disabled: true });
  });

  it('shows no placeholder when a value or all is selected', () => {
    const picked = filterOptions(choice({ values: ['en', 'es'], selected: 'es' }), 'language');
    expect(picked.some((o) => o.value === '' || o.disabled)).toBe(false);

    const all = filterOptions(choice({ values: ['en', 'es'], all: true, selected: 'all' }), 'language');
    expect(all.some((o) => o.value === '' || o.disabled)).toBe(false);
  });
});

describe('filterControl', () => {
  const listed = choice({ values: ['en'], selected: 'en' });

  it('is free text before any load', () => {
    expect(filterControl({ loaded: false, loadError: '' }, null)).toBe('text');
  });

  it('stays free text after a failed load of an unknown site, so the value can be retried', () => {
    expect(filterControl({ loaded: false, loadError: 'site not recognised' }, null)).toBe('text');
  });

  it('is disabled only when the loaded site lists none', () => {
    expect(filterControl({ loaded: true, loadError: '' }, null)).toBe('disabled');
  });

  it('is a selector whenever the site listed values, loaded or not', () => {
    expect(filterControl({ loaded: true, loadError: '' }, listed)).toBe('select');
    expect(filterControl({ loaded: false, loadError: 'network down' }, listed)).toBe('select');
  });
});

describe('typedValue and sameScanlator', () => {
  it('shows "all" in any case as empty, and other values as they are', () => {
    expect(typedValue('all')).toBe('');
    expect(typedValue('ALL')).toBe('');
    expect(typedValue('es')).toBe('es');
    expect(typedValue('')).toBe('');
  });

  it('treats "" and "all" as one scanlator filter', () => {
    expect(sameScanlator('', 'all')).toBe(true);
    expect(sameScanlator('Delta', 'all')).toBe(false);
  });
});
