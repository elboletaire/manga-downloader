import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import type { Catalog } from './index';
import { I18nProvider, T, useI18n } from './react';

const catalog: Catalog = {
  en: {
    'app.hint': 'Press <kbd>Ctrl+,</kbd> to open <em>Settings</em>, see <code>1-10,12</code>',
    'app.plain': 'Plain <b>bold</b> stays',
    'app.unclosed': 'Open <strong>tag',
    'app.saved': '{{count}} saved',
    'app.saved_one': '{{count}} file saved',
  },
  es: {
    'app.hint': 'Pulsa <kbd>Ctrl+,</kbd>',
  },
};

const persist = async () => {};

function provided(pref: '' | 'en' | 'es', child: ReturnType<typeof createElement>, languages = ['fr-FR']) {
  return renderToStaticMarkup(
    createElement(I18nProvider, { pref, persist, catalog, systemLanguages: languages }, child),
  );
}

describe('T', () => {
  it('turns known tags into elements', () => {
    const html = provided('en', createElement(T, { k: 'app.hint' }));
    expect(html).toBe(
      'Press <kbd>Ctrl+,</kbd> to open <em>Settings</em>, see <code>1-10,12</code>',
    );
  });

  it('leaves unknown tags as text and closes unclosed ones', () => {
    expect(provided('en', createElement(T, { k: 'app.plain' }))).toBe('Plain &lt;b&gt;bold&lt;/b&gt; stays');
    expect(provided('en', createElement(T, { k: 'app.unclosed' }))).toBe('Open tag');
  });

  it('takes custom components and values', () => {
    const html = provided(
      'en',
      createElement(T, {
        k: 'app.saved',
        values: { count: 1 },
      }),
    );
    expect(html).toBe('1 file saved');
    const custom = renderToStaticMarkup(
      createElement(
        I18nProvider,
        { pref: 'en', persist, catalog },
        createElement(T, { k: 'app.hint', components: { kbd: (c) => createElement('span', { className: 'key' }, c) } }),
      ),
    );
    expect(custom).toContain('<span class="key">Ctrl+,</span>');
  });

  it('shows the language the preference picks, and the catalog of that language', () => {
    expect(provided('es', createElement(T, { k: 'app.hint' }))).toBe('Pulsa <kbd>Ctrl+,</kbd>');
    expect(provided('', createElement(T, { k: 'app.hint' }), ['es-ES'])).toBe('Pulsa <kbd>Ctrl+,</kbd>');
  });
});

describe('useI18n', () => {
  function Probe() {
    const { lang, pref, systemLang, t } = useI18n();
    return createElement('span', null, `${lang}|${pref}|${systemLang}|${t('common.close')}`);
  }

  it('is English without a provider', () => {
    expect(renderToStaticMarkup(createElement(Probe))).toBe('<span>en||en|Close</span>');
  });

  it('reports the preference, the resolved language and the system language', () => {
    const provider = createElement(I18nProvider, { pref: 'ca', persist, systemLanguages: ['fr-FR'] }, createElement(Probe));
    expect(renderToStaticMarkup(provider)).toBe(
      '<span>ca|ca|en|Tanca</span>',
    );
  });
});
