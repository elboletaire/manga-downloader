import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { LANGS, type LangPref } from './i18n';
import { I18nProvider } from './i18n/react';
import { KOFI_URL, LICENSE_URL } from './links';
import { SettingsDialog, websiteURL } from './SettingsDialog';
import type { State } from './types';

const persist = async () => undefined;

type Overrides = Partial<{
  releaseURL: string;
  prefs: { uiLanguage: LangPref; checkUpdates: boolean; simpleStart: boolean };
}>;

function state(overrides: Overrides = {}) {
  return {
    version: 'v1.18.0',
    releaseURL: 'https://github.com/elboletaire/manga-downloader/releases/tag/v1.18.0',
    prefs: { uiLanguage: '', checkUpdates: true, simpleStart: true },
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
        onSimpleStart: () => undefined,
        onOpenURL: () => undefined,
        onError: () => undefined,
      }),
    ),
  );
}

// the update switch is the last one; the simple view's comes before it
const updateSwitch = (html: string) => html.match(/<input[^>]*role="switch"[^>]*>/g)?.at(-1) ?? '';

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

  it('shows the simple view switch as the pref says', () => {
    const first = (html: string) => html.match(/<input[^>]*role="switch"[^>]*>/)?.[0] ?? '';
    const on = render('en', state());
    expect(on).toContain('Open with the simple view');
    expect(first(on)).toContain('checked=""');
    const off = render('en', state({ prefs: { uiLanguage: '', checkUpdates: true, simpleStart: false } }));
    expect(first(off)).not.toContain('checked=""');
  });

  it('shows the update switch off and disabled with the dev hint when there is no release URL', () => {
    const html = render('en', state({ releaseURL: '' }));
    const input = updateSwitch(html);
    expect(html).toContain('Development builds don&#x27;t check for updates.');
    expect(html).not.toContain('Asks GitHub for the latest release');
    expect(input).toContain('disabled=""');
    expect(input).not.toContain('checked=""');
  });

  it('enables the update switch with its hint on a release build', () => {
    const html = render('en', state());
    const input = updateSwitch(html);
    expect(html).toContain('Checks for the latest release each time the app opens, and counts daily users without keeping your IP.');
    expect(html).not.toContain('Development builds');
    expect(input).not.toContain('disabled=""');
    expect(input).toContain('checked=""');
  });

  it('renders the dialog in Japanese, with the language picked', () => {
    const html = render('ja', state());
    expect(html).toContain('<h2 id="settings-title">設定</h2>');
    expect(html).toContain('<option value="ja" selected="">日本語</option>');
  });

  it('links the author alias and the license, untranslated, in each language', () => {
    const lines = { en: 'Made with love by', es: 'Hecho con amor por', ca: 'Fet amb amor per', ja: '作者:' };
    const author = '<a href="https://github.com/elboletaire">elboletaire</a>';
    const license = `<a href="${LICENSE_URL}">AGPL-3.0</a>`;
    for (const lang of LANGS) {
      const html = render(lang, state());
      expect(html, lang).toContain(`${lines[lang]} ${author} · © 2023–2026 · ${license}`);
    }
  });

  it('links the Ko-fi page with its cup, named in each language', () => {
    const names = {
      en: 'Support me on Ko-fi',
      es: 'Apóyame en Ko-fi',
      ca: 'Dona&#x27;m suport a Ko-fi',
      ja: 'Ko-fiで応援する',
    };
    for (const lang of LANGS) {
      const html = render(lang, state());
      expect(html, lang).toContain(`<a class="kofi" href="${KOFI_URL}"`);
      expect(html, lang).toContain(names[lang]);
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
    expect(websiteURL('ja')).toBe('https://mangadownloader.cat/ja/');
  });
});
