import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { createMockApi } from './mock';
import { SourceBar } from './SourceBar';
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
      onClear: () => undefined,
      onOptions: () => Promise.resolve(),
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
