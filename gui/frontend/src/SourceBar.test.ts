import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { I18nProvider } from './i18n/react';
import { createMockApi } from './mock';
import { pasteValue, SourceBar } from './SourceBar';
import { isMac, shortcutLabel } from './settingsShortcut';
import type { State } from './types';

async function noneSite(language: string): Promise<State> {
  const base = await createMockApi({ scenario: 'loaded', site: 'none', freeze: true }).state();

  return { ...base, options: { ...base.options, language }, loadedLanguage: language };
}

function render(state: State): string {
  return renderToStaticMarkup(
    createElement(SourceBar, {
      state,
      onLoad: () => undefined,
      readClipboard: () => Promise.resolve(''),
      onClear: () => undefined,
      onOptions: () => Promise.resolve(),
      onPick: () => Promise.resolve(),
      onError: () => undefined,
      loadRefusal: '',
      optionError: '',
      onOpenSettings: () => undefined,
      settingsOpen: false,
    }),
  );
}

describe('SourceBar filters for a site that lists no languages', () => {
  it('shows the language field disabled and cleared, with the hint', async () => {
    const markup = render(await noneSite('ca'));

    expect(markup).toContain('This site has no language choice');
    expect(markup).not.toContain('value="ca"');
    expect(markup).toMatch(/<input[^>]*disabled[^>]*\/>/);
  });
});

describe('SourceBar free-text filters', () => {
  it('shows a stored "all" as the empty field with the "any" placeholder', async () => {
    const base = await createMockApi({ scenario: 'loaded', freeze: true }).state();
    const state: State = { ...base, loaded: false, loadError: '', options: { ...base.options, language: 'all', scanlator: 'All' } };
    const markup = render(state);

    expect(markup).not.toMatch(/value="[Aa]ll"/);
    expect(markup.match(/placeholder="any"/g)?.length).toBe(2);
  });

  it('keeps a typed value as text', async () => {
    const base = await createMockApi({ scenario: 'loaded', freeze: true }).state();
    const markup = render({ ...base, loaded: false, loadError: '', options: { ...base.options, language: 'es' } });

    expect(markup).toContain('value="es"');
  });
});

describe('pasteValue', () => {
  it('trims the clipboard text', () => {
    expect(pasteValue('  https://example.com/manga/one-piece\n')).toBe('https://example.com/manga/one-piece');
  });

  it('gives nothing for an empty or blank clipboard', () => {
    expect(pasteValue('')).toBe('');
    expect(pasteValue(' \t\n')).toBe('');
  });

  it('passes text that is not a URL through, as typing would', () => {
    expect(pasteValue(' one piece ')).toBe('one piece');
  });
});

describe('SourceBar paste button', () => {
  it('shows in an empty box', async () => {
    const markup = render(await createMockApi({ scenario: 'empty', freeze: true }).state());

    expect(markup).toContain('aria-label="Paste link from clipboard"');
    expect(markup).not.toContain('class="url-clear"');
  });

  it('is replaced by the clear button once the box has text', async () => {
    const markup = render(await createMockApi({ scenario: 'loaded', freeze: true }).state());

    expect(markup).not.toContain('aria-label="Paste link from clipboard"');
    expect(markup).toContain('class="url-clear"');
  });
});

describe('SourceBar filter notes', () => {
  it('says a pick applies at once once the series is loaded', async () => {
    const base = await createMockApi({ scenario: 'loaded', freeze: true }).state();

    expect(render({ ...base, stale: false })).toContain('Picking one applies it at once');
    expect(render({ ...base, stale: false })).not.toContain('Not applied yet');
  });

  it('shows the not-applied note when the loaded list is stale', async () => {
    const base = await createMockApi({ scenario: 'loaded', freeze: true }).state();

    expect(render({ ...base, stale: true })).toContain('Not applied yet');
  });
});

describe('SourceBar in another language', () => {
  async function renderIn(lang: 'es' | 'ca', state: State): Promise<string> {
    return renderToStaticMarkup(
      createElement(
        I18nProvider,
        { pref: lang, persist: () => Promise.resolve() },
        createElement(SourceBar, {
          state,
          onLoad: () => undefined,
          readClipboard: () => Promise.resolve(''),
          onClear: () => undefined,
          onOptions: () => Promise.resolve(),
          onPick: () => Promise.resolve(),
          onError: () => undefined,
          loadRefusal: '',
          optionError: '',
          onOpenSettings: () => undefined,
          settingsOpen: false,
        }),
      ),
    );
  }

  it('shows the Spanish labels and the Spanish language names', async () => {
    const empty = await renderIn('es', await createMockApi({ scenario: 'empty', freeze: true }).state());
    expect(empty).toContain('aria-label="Pegar el enlace del portapapeles"');
    expect(empty).toContain('Filtrar la lista');
    expect(empty).toContain('>Cargar<');

    const loaded = await renderIn('es', await createMockApi({ scenario: 'loaded', freeze: true }).state());
    expect(loaded).toContain('Recargar');
  });

  it('shows the Catalan labels', async () => {
    const markup = await renderIn('ca', await createMockApi({ scenario: 'loaded', freeze: true }).state());
    expect(markup).toContain('Filtra la llista');
    expect(markup).toContain('Recarrega');
  });
});

describe('SourceBar settings button', () => {
  it('shows the gear with the shortcut in its label, and the dialog state', async () => {
    const base = await createMockApi({ scenario: 'empty', freeze: true }).state();
    const markup = renderToStaticMarkup(
      createElement(SourceBar, {
        state: base,
        onLoad: () => undefined,
        readClipboard: () => Promise.resolve(''),
        onClear: () => undefined,
        onOptions: () => Promise.resolve(),
        onPick: () => Promise.resolve(),
        onError: () => undefined,
        loadRefusal: '',
        optionError: '',
        onOpenSettings: () => undefined,
        settingsOpen: true,
      }),
    );
    const label = `Settings (${shortcutLabel(isMac())})`;

    expect(markup).toContain('settings-btn');
    expect(markup).toContain('aria-haspopup="dialog"');
    expect(markup).toContain('aria-expanded="true"');
    expect(markup).toContain(`aria-label="${label}"`);
  });

  it('translates its label to Spanish', async () => {
    const base = await createMockApi({ scenario: 'empty', freeze: true }).state();
    const markup = renderToStaticMarkup(
      createElement(
        I18nProvider,
        { pref: 'es', persist: () => Promise.resolve() },
        createElement(SourceBar, {
          state: base,
          onLoad: () => undefined,
          readClipboard: () => Promise.resolve(''),
          onClear: () => undefined,
          onOptions: () => Promise.resolve(),
          onPick: () => Promise.resolve(),
          onError: () => undefined,
          loadRefusal: '',
          optionError: '',
          onOpenSettings: () => undefined,
          settingsOpen: false,
        }),
      ),
    );

    expect(markup).toContain(`aria-label="Ajustes (${shortcutLabel(isMac())})"`);
    expect(markup).toContain('aria-expanded="false"');
  });
});
