import { describe, expect, it } from 'vitest';
import { createMockApi, scenarioFrom } from './mock';

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
    const api = createMockApi('empty');
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

  it('starts finished with a summary for the done scenario', async () => {
    const api = createMockApi('done');
    const state = await api.state();
    expect(state.summary?.failures).toHaveLength(1);
    expect(state.running).toBe(false);
  });
});
