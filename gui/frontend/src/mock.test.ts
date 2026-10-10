import { describe, expect, it } from 'vitest';
import { createMockApi, mockSetupFrom, scenarioFrom } from './mock';

describe('scenarioFrom', () => {
  it('reads the scenario from the query, defaulting to empty', () => {
    expect(scenarioFrom('?mock&scenario=running')).toBe('running');
    expect(scenarioFrom('?scenario=bogus')).toBe('empty');
    expect(scenarioFrom('')).toBe('empty');
  });
});

describe('createMockApi', () => {
  it('starts empty and refuses a download without a series', async () => {
    const api = createMockApi('empty');
    expect((await api.state()).loaded).toBe(false);
    await expect(api.start()).rejects.toBe('load a series first');
  });

  it('loads a series, selects a range and reports the match count', async () => {
    const api = createMockApi({ chapters: 30 });
    await api.load('https://example.com/series/a');
    const state = await api.state();
    expect(state.loaded).toBe(true);
    expect(state.chapterCount).toBe(30);

    expect(await api.selectRange('1-3,5')).toBe(4);
    expect(await api.selection()).toEqual([0, 1, 2, 4]);
    expect(await api.selectRange('99')).toBe(0);
  });

  it('rejects an empty selection and an empty URL', async () => {
    const api = createMockApi('loaded');
    await api.setAll(false);
    await expect(api.start()).rejects.toBe('select at least one chapter');
    await expect(api.load('  ')).rejects.toBe('enter the URL of a series first');
  });

  it('marks the list stale when the language changes until it is reloaded', async () => {
    const api = createMockApi('loaded');
    await api.setOptions({ ...(await api.state()).options, language: 'es' });
    expect((await api.state()).stale).toBe(true);
    await api.load((await api.state()).url);
    expect((await api.state()).stale).toBe(false);
  });

  it('selects a set of indices in one call, or none when one is out of range', async () => {
    const api = createMockApi({ chapters: 10 });
    await api.load('https://example.com/series/a');
    await api.setSelectedMany([2, 3, 4], true);
    expect(await api.selection()).toEqual([2, 3, 4]);

    await expect(api.setSelectedMany([0, 99], true)).rejects.toBe('chapter 99 is not in the list');
    expect(await api.selection()).toEqual([2, 3, 4]);

    await api.setSelectedMany([3], false);
    expect(await api.selection()).toEqual([2, 4]);
  });

  it('refuses to change the selection, pick a folder or start while a download runs', async () => {
    const api = createMockApi({ scenario: 'running', freeze: true });
    const busy = 'a series is loading or a download is running';
    await expect(api.setSelected(0, false)).rejects.toBe(busy);
    await expect(api.setSelectedMany([0], false)).rejects.toBe(busy);
    await expect(api.setAll(false)).rejects.toBe(busy);
    await expect(api.selectRange('1')).rejects.toBe(busy);
    await expect(api.chooseFolder()).rejects.toBe(busy);
    await expect(api.start()).rejects.toBe(busy);
  });

  it('refuses to start while a series loads', async () => {
    const api = createMockApi({ scenario: 'loading', freeze: true });
    await expect(api.start()).rejects.toBe('a series is loading or a download is running');
  });

  it('keeps the bundle flag a run started with', async () => {
    const api = createMockApi('done');
    expect((await api.state()).run.bundle).toBe(false);
    await api.setOptions({ ...(await api.state()).options, bundle: true });
    expect((await api.state()).run.bundle).toBe(false);
  });

  it('reports the loaded language and scanlator, apart from the options', async () => {
    const api = createMockApi('loaded');
    await api.setOptions({ ...(await api.state()).options, language: 'es' });
    const state = await api.state();
    expect(state.loadedLanguage).toBe('');
    expect(state.options.language).toBe('es');
    expect(state.loadedScanlator).toBe(state.options.scanlator);
  });

  it('starts finished with a summary for the done scenario', async () => {
    const api = createMockApi('done');
    const state = await api.state();
    expect(state.summary?.failures).toHaveLength(1);
    expect(state.summary?.skipped).toBe(1);
    expect(state.running).toBe(false);
  });

  it('starts mid-run with mixed job states, and freezes on request', async () => {
    const api = createMockApi({ scenario: 'running', freeze: true });
    const state = await api.state();
    expect(state.running).toBe(true);
    const states = new Set(state.jobs.map((j) => j.state));
    expect(states.has('done')).toBe(true);
    expect(states.has('failed')).toBe(true);
    // jobs only exist for started chapters, as in the backend
    expect(state.jobs.length).toBeLessThan(state.run.chapters);
  });

  it('runs a bundle as a single job', async () => {
    const state = await createMockApi({ scenario: 'bundle', freeze: true }).state();
    expect(state.run.bundle).toBe(true);
    expect(state.jobs).toHaveLength(1);
    expect(state.jobs[0].status).not.toBe('');
  });

  it('has a load error and nothing listed for the error scenario', async () => {
    const state = await createMockApi('error').state();
    expect(state.loaded).toBe(false);
    expect(state.loadError).not.toBe('');
  });

  it('is stale for the stale scenario', async () => {
    const state = await createMockApi('stale').state();
    expect(state.stale).toBe(true);
    expect(state.selectedCount).toBeGreaterThan(0);
  });

  it('lists some numbers once per language with dupes', async () => {
    const api = createMockApi({ scenario: 'loaded', dupes: true, chapters: 10 });
    const state = await api.state();
    expect(state.hasDuplicates).toBe(true);
    const tens = (await api.chapters()).filter((c) => c.number === 10);
    expect(tens.map((c) => c.language)).toEqual(['en', 'es']);
  });
});

describe('mockSetupFrom', () => {
  it('reads the flags and the chapter count', () => {
    expect(mockSetupFrom('?scenario=bundle&freeze&dupes&chapters=50')).toEqual({
      scenario: 'bundle',
      freeze: true,
      dupes: true,
      chapters: 50,
      site: 'multi',
    });
    expect(mockSetupFrom('?chapters=nope').chapters).toBeGreaterThan(100);
  });

  it('reads the site, defaulting to multi', () => {
    expect(mockSetupFrom('?site=plus').site).toBe('plus');
    expect(mockSetupFrom('?site=bogus').site).toBe('multi');
  });
});

describe('mock filters', () => {
  it('offers the languages of the site, and none for a site without them', async () => {
    const multi = createMockApi({ scenario: 'loaded', site: 'multi' });
    expect((await multi.state()).languages).toEqual({ values: ['en', 'es', 'ja'], all: true, selected: 'all' });
    expect((await multi.state()).scanlators).toBeNull();

    const none = createMockApi({ scenario: 'loaded', site: 'none' });
    expect((await none.state()).languages).toBeNull();
    expect((await none.state()).scanlators).toBeNull();
  });

  it('shows the default edition of a site with no All, and the largest group of an Atsumaru-like one', async () => {
    const plus = createMockApi({ scenario: 'loaded', site: 'plus' });
    expect((await plus.state()).languages).toEqual({ values: ['en', 'es'], all: false, selected: 'en' });

    const groups = createMockApi({ scenario: 'loaded', site: 'groups' });
    expect((await groups.state()).scanlators).toEqual({ values: ['Alpha', 'Delta'], all: true, selected: 'Delta' });
  });

  it('lists no filters before a load', async () => {
    const api = createMockApi({ scenario: 'empty', site: 'multi' });
    expect((await api.state()).languages).toBeNull();
  });

  it('falls back to all on a language the series lacks, and logs it', async () => {
    const api = createMockApi({ scenario: 'loaded', site: 'multi' });
    await api.setOptions({ ...(await api.state()).options, language: 'xx' });
    await api.load((await api.state()).url);

    const state = await api.state();
    expect(state.options.language).toBe('all');
    expect(state.loadedLanguage).toBe('all');
    expect(state.stale).toBe(false);
    expect((await api.log()).map((l) => l.text)).toContain(
      'language "xx" isn\'t in this series, loading all of them instead',
    );
  });

  it('falls back to the default edition where the site has no all', async () => {
    const api = createMockApi({ scenario: 'loaded', site: 'plus' });
    await api.setOptions({ ...(await api.state()).options, language: 'xx' });
    await api.load((await api.state()).url);

    expect((await api.state()).options.language).toBe('');
    expect((await api.state()).languages?.selected).toBe('en');
  });
});
