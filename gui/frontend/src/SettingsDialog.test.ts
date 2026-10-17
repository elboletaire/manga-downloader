import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { LANGS, type LangPref } from './i18n';
import { I18nProvider } from './i18n/react';
import { LICENSE_URL, SettingsDialog, websiteURL } from './SettingsDialog';
import type { State } from './types';

const persist = async () => undefined;

type Overrides = Partial<{ releaseURL: string; prefs: { uiLanguage: LangPref; checkUpdates: boolean } }>;

function state(overrides: Overrides = {}) {
  return {
    version: 'v1.18.0',
    releaseURL: 'https://github.com/elboletaire/manga-downloader/releases/tag/v1.18.0',
    prefs: { uiLanguage: '', checkUpdates: true },
    ...overrides,
  } as unknown as State;
}

function render(pref: LangPref, s: State, systemLanguages = ['fr-FR']) {
  return renderToStaticMarkup(
    createElement(
      I18nProvider,
      { pref, persist, systemLanguages },
      createElement(SettingsDialog, {
        state: s,
        onClose: () => undefined,
        onCheckUpdates: () => undefined,
        onOpenURL: () => undefined,
        onError: () => undefined,
      }),
    ),
  );
}

describe('SettingsDialog', () => {
  it('names each language in itself as an option, whatever the UI language', () => {
    for (const lang of LANGS) {
      const html = render(lang, state());
      expect(html, lang).toContain('>English</option>');
      expect(html, lang).toContain('>Español</option>');
      expect(html, lang).toContain('>Català</option>');
    }
  });

  it('names the resolved system language in the System option', () => {
    expect(render('', state(), ['fr-FR'])).toContain('<option value="" selected="">System (English)</option>');
    expect(render('', state(), ['es-ES'])).toContain('<option value="" selected="">Sistema (Español)</option>');
    expect(render('', state(), ['ca-ES'])).toContain('<option value="" selected="">Sistema (Català)</option>');
  });

  it('selects the option matching the saved language', () => {
    for (const lang of LANGS) {
      const html = render(lang, state());
      expect(html, lang).toContain(`<option value="${lang}" selected="">`);
      expect(html.match(/selected=""/g), lang).toHaveLength(1);
    }
    expect(render('', state())).not.toContain('<option value="en" selected="">');
  });

  it('shows the update switch off and disabled with the dev hint when there is no release URL', () => {
    const html = render('en', state({ releaseURL: '' }));
    const input = html.match(/<input[^>]*role="switch"[^>]*>/)?.[0] ?? '';
    expect(html).toContain('Development builds don&#x27;t check for updates.');
    expect(html).not.toContain('Asks GitHub for the latest release');
    expect(input).toContain('disabled=""');
    expect(input).not.toContain('checked=""');
  });

  it('enables the update switch with its hint on a release build', () => {
    const html = render('en', state());
    const input = html.match(/<input[^>]*role="switch"[^>]*>/)?.[0] ?? '';
    expect(html).toContain('Asks GitHub for the latest release each time the app opens.');
    expect(html).not.toContain('Development builds');
    expect(input).not.toContain('disabled=""');
    expect(input).toContain('checked=""');
  });

  it('links the author alias and the license, untranslated, in each language', () => {
    const lines = { en: 'Made with love by', es: 'Hecho con amor por', ca: 'Fet amb amor per' };
    const author = '<a href="https://github.com/elboletaire">elboletaire</a>';
    const license = `<a href="${LICENSE_URL}">AGPL-3.0</a>`;
    for (const lang of LANGS) {
      const html = render(lang, state());
      expect(html, lang).toContain(`${lines[lang]} ${author} · © 2023–2026 · ${license}`);
    }
  });

  it('links the version to the release only when there is one', () => {
    const release = 'https://github.com/elboletaire/manga-downloader/releases/tag/v1.18.0';
    expect(render('en', state())).toContain(`href="${release}"`);
    expect(render('en', state({ releaseURL: '' }))).not.toContain('/releases/tag/');
  });
});

describe('websiteURL', () => {
  it('points each language at its own page of the website', () => {
    expect(websiteURL('en')).toBe('https://mangadownloader.cat/');
    expect(websiteURL('es')).toBe('https://mangadownloader.cat/es/');
    expect(websiteURL('ca')).toBe('https://mangadownloader.cat/ca/');
  });
});
