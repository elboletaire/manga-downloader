import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import type { Api } from './api';
import type { LangPref } from './i18n';
import { I18nProvider } from './i18n/react';
import { App } from './App';
import { createMockApi } from './mock';
import { Root, watchPrefs } from './Root';
import type { State } from './types';

/** Watches the saved language with the mock; `saved` collects every value it reports */
function watch(api: Api) {
  const saved: LangPref[] = [];
  const initial = new Promise<State | null>((resolve) => {
    watchPrefs(api, { onSaved: (pref) => saved.push(pref), onInitial: resolve });
  });
  return { saved, initial };
}

/** The window as the Root shows it once the saved language is known */
const windowIn = (pref: LangPref, initial: State) =>
  renderToStaticMarkup(
    createElement(
      I18nProvider,
      { pref, persist: async () => {}, systemLanguages: ['fr-FR'] },
      createElement(App, { api: {} as Api, initial }),
    ),
  );

describe('Root', () => {
  it('renders nothing before the first snapshot, so no system language shows first', () => {
    expect(renderToStaticMarkup(createElement(Root, { api: createMockApi({ uiLanguage: 'ca' }) }))).toBe('');
  });

  it('gives the window the saved language from the first snapshot', async () => {
    const { saved, initial } = watch(createMockApi({ uiLanguage: 'ca' }));
    const state = await initial;
    expect(saved).toEqual(['ca']);
    expect(windowIn(saved.at(-1)!, state!)).toContain('Comença amb una sèrie');
  });

  it('switches the language when a push carries a new one', async () => {
    const api = createMockApi({ uiLanguage: 'ca' });
    const { saved, initial } = watch(api);
    const state = await initial;
    await api.setUILanguage('es');
    expect(saved.at(-1)).toBe('es');
    expect(windowIn(saved.at(-1)!, state!)).toContain('Empieza con una serie');
  });

  it('reports no first snapshot when it cannot be read, so the window asks again', async () => {
    const failing = { ...createMockApi(), state: () => Promise.reject(new Error('no backend')) } as unknown as Api;
    const { initial } = watch(failing);
    expect(await initial).toBeNull();
  });
});
