import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { I18nProvider } from './i18n/react';
import { LogDrawer, StatusBar } from './LogPanel';
import type { LogLine, Update } from './types';

const RELEASE = 'https://github.com/elboletaire/manga-downloader/releases/tag/v1.12.0';

function bar(overrides: { releaseURL?: string; update?: Update | null; log?: LogLine[] } = {}): string {
  return renderToStaticMarkup(
    createElement(StatusBar, {
      log: [],
      version: 'v1.12.0',
      releaseURL: RELEASE,
      update: null,
      open: false,
      onToggle: () => undefined,
      onOpenURL: () => undefined,
      onUpdate: () => Promise.resolve(),
      ...overrides,
    }),
  );
}

describe('StatusBar version', () => {
  it('links a release build to its release, with no update badge when up to date', () => {
    const html = bar();
    expect(html).toContain(`href="${RELEASE}"`);
    expect(html).toContain('class="version"');
    expect(html).not.toContain('update-badge');
  });

  it('shows a dev build as plain text, without a link', () => {
    const html = bar({ releaseURL: '' });
    expect(html).not.toContain('href=');
    expect(html).toContain('<span class="version" title="Version">v1.12.0</span>');
  });

  it('flags a newer release in yellow, before the version', () => {
    const html = bar({
      update: { latest: 'v1.13.0', url: 'https://github.com/elboletaire/manga-downloader/releases/latest', oneClick: false },
    });
    expect(html).toContain('class="update-badge warning"');
    expect(html).toContain('aria-label="Update available: v1.13.0"');
    expect(html).toContain('href="https://github.com/elboletaire/manga-downloader/releases/latest"');
    expect(html.indexOf('update-badge')).toBeLessThan(html.indexOf('class="version"'));
  });

  it('offers the one-click update only when the backend says it can', () => {
    const url = 'https://github.com/elboletaire/manga-downloader/releases/latest';
    const html = bar({ update: { latest: 'v1.13.0', url, oneClick: true } });
    expect(html).toContain('update-badge');
    expect(html).not.toContain('Update to');
    // the dialog opens on click, so it isn't in the first render
    expect(html).not.toContain('alertdialog');
  });
});

describe('StatusBar last-line title', () => {
  it('sets a title attribute on the log preview so the full text is visible on hover', () => {
    const text =
      'site check: timed out waiting for selector at https://example.com/manga/the-cartographer-of-lost-harbours';
    const html = bar({ log: [{ level: 'warn', text }] });
    expect(html).toContain(`title="${text}"`);
  });

  it('omits the title attribute when no log line has been emitted yet', () => {
    const html = bar({ log: [] });
    // the "Ready" placeholder has no tooltip — there is nothing to expand
    expect(html).toContain('aria-live="polite">Ready</span>');
  });
});

describe('StatusBar update prompt', () => {
  const update: Update = { latest: 'v1.18.0', url: 'https://example.com/latest', oneClick: true };

  it('opens the dialog on load only when the update asks for it', () => {
    expect(bar({ update })).not.toContain('update-dialog-title');
    const html = bar({ update: { ...update, prompt: true } });
    expect(html).toContain('update-dialog-title');
    expect(html).toContain('v1.18.0');
  });
});

describe('StatusBar translations', () => {
  const render = (lang: 'es' | 'ca', log: LogLine[]) =>
    renderToStaticMarkup(
      createElement(
        I18nProvider,
        { pref: lang, persist: async () => undefined, systemLanguages: ['en'] },
        createElement(StatusBar, {
          log,
          version: 'v1.12.0',
          releaseURL: '',
          update: { latest: 'v1.13.0', url: 'https://example.com/latest', oneClick: false },
          open: false,
          onToggle: () => undefined,
          onOpenURL: () => undefined,
          onUpdate: () => Promise.resolve(),
        }),
      ),
    );

  it('shows the words and the plural counts in the chosen language', () => {
    const html = render('es', [
      { level: 'error', text: 'boom' },
      { level: 'warn', text: 'careful' },
      { level: 'warn', text: 'again' },
    ]);
    expect(html).toContain('Registro');
    expect(html).toContain('aria-label="1 error"');
    expect(html).toContain('aria-label="2 advertencias"');
    expect(html).toContain('aria-label="Actualización disponible: v1.13.0"');
    expect(render('es', [])).toContain('>Listo</span>');
  });

  it('translates the log toggle and the Catalan words', () => {
    const html = render('ca', []);
    expect(html).toContain('title="Mostra el registre"');
    expect(html).toContain('>Preparat</span>');
  });
});

describe('LogDrawer messages', () => {
  const drawer = (log: LogLine[]) =>
    renderToStaticMarkup(
      createElement(
        I18nProvider,
        { pref: 'es', persist: async () => undefined, systemLanguages: ['en'] },
        createElement(LogDrawer, { log, onClose: () => undefined }),
      ),
    );

  it('shows a message line in the chosen language, and the plain text without one', () => {
    const html = drawer([
      {
        level: 'info',
        text: 'Loaded Naruto: 2 chapters',
        msg: { key: 'log.loaded', args: { title: 'Naruto', count: 2 }, text: 'Loaded Naruto: 2 chapters' },
      },
      { level: 'warn', text: 'site check: timed out' },
    ]);
    expect(html).toContain('Se ha cargado Naruto: 2 capítulos');
    expect(html).toContain('site check: timed out');
    expect(html).not.toContain('Loaded Naruto');
  });
});
