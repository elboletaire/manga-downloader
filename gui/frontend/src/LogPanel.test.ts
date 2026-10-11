import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { StatusBar } from './LogPanel';
import type { Update } from './types';

const RELEASE = 'https://github.com/elboletaire/manga-downloader/releases/tag/v1.12.0';

function bar(overrides: { releaseURL?: string; update?: Update | null } = {}): string {
  return renderToStaticMarkup(
    createElement(StatusBar, {
      log: [],
      version: 'v1.12.0',
      releaseURL: RELEASE,
      update: null,
      open: false,
      onToggle: () => undefined,
      onOpenURL: () => undefined,
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
      update: { latest: 'v1.13.0', url: 'https://github.com/elboletaire/manga-downloader/releases/latest' },
    });
    expect(html).toContain('class="update-badge warning"');
    expect(html).toContain('aria-label="Update available: v1.13.0"');
    expect(html).toContain('href="https://github.com/elboletaire/manga-downloader/releases/latest"');
    expect(html.indexOf('update-badge')).toBeLessThan(html.indexOf('class="version"'));
  });

});
