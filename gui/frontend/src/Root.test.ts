import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Api } from './api';
import { App } from './App';
import { I18nProvider } from './i18n/react';
import { createMockApi } from './mock';
import { prefOf, reportSystemLocale, Root, systemLocale, watchState } from './Root';
import type { State } from './types';

// lets the snapshot and any pushed State settle
const settle = () => new Promise((resolve) => setTimeout(resolve, 0));

/** Watches the window's State with the mock; `states` collects every State it is given */
function collect(api: Api) {
  const states: State[] = [];
  let failed = false;
  const stop = watchState(api, {
    onState: (state) => states.push(state),
    onFailure: () => {
      failed = true;
    },
  });
  return { states, failed: () => failed, stop };
}

/** The window as the Root shows it once the State is known */
const windowIn = (state: State) =>
  renderToStaticMarkup(
    createElement(
      I18nProvider,
      { pref: prefOf(state), persist: async () => {}, systemLanguages: ['fr-FR'] },
      createElement(App, { api: {} as Api, initial: state }),
    ),
  );

describe('reportSystemLocale', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('reports the first of the system languages, once', () => {
    vi.stubGlobal('navigator', { languages: ['fr-FR', 'en-US'], language: 'fr-FR' });
    const setSystemLocale = vi.fn(async () => {});
    reportSystemLocale({ setSystemLocale } as unknown as Api);
    expect(setSystemLocale).toHaveBeenCalledTimes(1);
    expect(setSystemLocale).toHaveBeenCalledWith('fr-FR');
  });

  it('falls back to navigator.language when there are no languages', () => {
    vi.stubGlobal('navigator', { language: 'ca-ES' });
    expect(systemLocale()).toBe('ca-ES');
  });
});

describe('Root', () => {
  it('renders nothing before the first State, so no system language shows first', () => {
    expect(renderToStaticMarkup(createElement(Root, { api: createMockApi({ uiLanguage: 'ca', simpleStart: false }) }))).toBe('');
  });

  it('gives the window the saved language from the first snapshot', async () => {
    const watched = collect(createMockApi({ uiLanguage: 'ca', simpleStart: false }));
    await settle();
    expect(watched.states).toHaveLength(1);
    expect(windowIn(watched.states[0])).toContain('Comença amb una sèrie');
    watched.stop();
  });

  it('switches the language when a push carries a new one', async () => {
    const api = createMockApi({ uiLanguage: 'ca', simpleStart: false });
    const watched = collect(api);
    await settle();
    await api.setUILanguage('es');
    const newest = watched.states.at(-1)!;
    expect(prefOf(newest)).toBe('es');
    expect(windowIn(newest)).toContain('Empieza con una serie');
    watched.stop();
  });

  it('keeps a push that arrives before the snapshot resolves, and ignores the older snapshot', async () => {
    const base = createMockApi({ uiLanguage: 'en' });
    const older = await base.state();
    let resolveSnapshot: (state: State) => void = () => {};
    const api: Api = { ...base, state: () => new Promise<State>((resolve) => (resolveSnapshot = resolve)) };

    const watched = collect(api);
    await base.setUILanguage('es');
    resolveSnapshot(older);
    await settle();

    expect(watched.states).toHaveLength(1);
    expect(prefOf(watched.states[0])).toBe('es');
    watched.stop();
  });

  it('reports a failed first snapshot when no push came, so the window asks again', async () => {
    const base = createMockApi();
    const failing: Api = { ...base, state: () => Promise.reject(new Error('no backend')) };
    const watched = collect(failing);
    await settle();
    expect(watched.failed()).toBe(true);
    expect(watched.states).toHaveLength(0);
    watched.stop();
  });
});
