import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import type { Api } from './api';
import { I18nProvider } from './i18n/react';
import { App } from './App';
import { createMockApi } from './mock';

// Without a snapshot the window shows its boot screen, so the test needs no API calls.
const boot = (pref: '' | 'en' | 'es' | 'ca') =>
  renderToStaticMarkup(
    createElement(
      I18nProvider,
      { pref, persist: async () => {}, systemLanguages: ['fr-FR'] },
      createElement(App, { api: {} as Api }),
    ),
  );

describe('App boot screen', () => {
  it('says Starting in English by default', () => {
    expect(boot('en')).toContain('Starting…');
  });

  it('translates the boot screen', () => {
    expect(boot('es')).toContain('Iniciando…');
    expect(boot('ca')).toContain('Iniciant…');
  });
});

describe('App with the first snapshot', () => {
  it('renders the snapshot it is given, in the saved language, with no boot screen', async () => {
    const initial = await createMockApi({ uiLanguage: 'ca', simpleStart: false }).state();
    const html = renderToStaticMarkup(
      createElement(
        I18nProvider,
        { pref: 'ca', persist: async () => {}, systemLanguages: ['fr-FR'] },
        createElement(App, { api: {} as Api, initial }),
      ),
    );
    expect(html).toContain('Comença amb una sèrie');
    expect(html).not.toContain('Iniciant…');
  });
});

describe('App start view', () => {
  const render = (initial: Awaited<ReturnType<Api['state']>>) =>
    renderToStaticMarkup(
      createElement(
        I18nProvider,
        { pref: 'en', persist: async () => {}, systemLanguages: ['fr-FR'] },
        createElement(App, { api: {} as Api, initial }),
      ),
    );

  it('opens an empty window on the simple view: the URL box and settings, nothing else', async () => {
    const html = render(await createMockApi('empty').state());
    expect(html).toContain('class="app simple"');
    expect(html).toContain('class="url-input"');
    expect(html).toContain('<span>Settings</span>');
    expect(html).not.toContain('class="workspace"');
    expect(html).not.toContain('class="filter-row"');
    expect(html).not.toContain('class="statusbar"');
  });

  it('opens on the full view with the pref off or a series loaded', async () => {
    for (const initial of [
      await createMockApi({ simpleStart: false }).state(),
      await createMockApi('loaded').state(),
    ]) {
      const html = render(initial);
      expect(html).toContain('class="app"');
      expect(html).toContain('class="workspace"');
    }
  });
});
