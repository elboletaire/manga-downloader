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
  it('names each language in itself, whatever the UI language', () => {
    for (const lang of LANGS) {
      const html = render(lang, state());
      expect(html, lang).toContain('English');
      expect(html, lang).toContain('Español');
      expect(html, lang).toContain('Català');
    }
  });

  it('shows the system hint only when the language follows the system', () => {
    expect(render('', state(), ['fr-FR'])).toContain('Follows the system: English.');
    expect(render('', state(), ['es-ES'])).toContain('Sigue el idioma del sistema: Español.');
    expect(render('en', state(), ['es-ES'])).not.toContain('Follows the system');
    expect(render('ca', state(), ['es-ES'])).not.toContain("Segueix l'idioma del sistema");
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

  it('credits the author and links the license, untranslated, in each language', () => {
    const lines = { en: 'Proudly made by', es: 'Hecho con orgullo por', ca: 'Fet amb orgull per' };
    for (const lang of LANGS) {
      const html = render(lang, state());
      expect(html, lang).toContain(`${lines[lang]} Òscar Casajuana Alonso · © 2023–2026 · <a href="${LICENSE_URL}"`);
      expect(html, lang).toContain('>AGPL-3.0</a>');
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
