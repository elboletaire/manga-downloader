import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { createMockApi } from './mock';
import { pasteValue, SourceBar } from './SourceBar';
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
