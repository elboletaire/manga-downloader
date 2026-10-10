import { describe, expect, it } from 'vitest';
import { filterControl, filtersIdentified, filterOptions, languageName } from './filters';
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

  it('does not repeat a listed selected value', () => {
    const options = filterOptions(choice({ values: ['en', 'es'], selected: 'es' }), 'language');
    expect(options.map((o) => o.value)).toEqual(['en', 'es']);
  });

  it('names the site default when it shows no single value', () => {
    const options = filterOptions(choice({ values: ['Alpha', 'Delta'], selected: '' }), 'scanlator');
    expect(options[0]).toEqual({ value: '', label: "The site's default" });
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
