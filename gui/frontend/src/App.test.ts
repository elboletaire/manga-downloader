import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import type { Api } from './api';
import { I18nProvider } from './i18n/react';
import { App } from './App';

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
