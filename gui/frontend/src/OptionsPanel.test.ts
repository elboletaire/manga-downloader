import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { createMockApi } from './mock';
import { AVIF_EPUB_HINT } from './convertImages';
import { OptionsPanel, PACK_HINTS, VOLUMES_HINT_NOT_LOADED } from './OptionsPanel';
import type { State } from './types';

function options(state: State): string {
  return renderToStaticMarkup(
    createElement(OptionsPanel, {
      state,
      onOptions: () => Promise.resolve(),
      onChooseFolder: () => Promise.resolve(),
      onError: () => undefined,
    }),
  );
}

describe('Pack as volumes hint', () => {
  it('shows the not-loaded hint when volumes is selected before a series loads', async () => {
    const base = await createMockApi({ scenario: 'empty', freeze: true }).state();
    expect(base.loaded).toBe(false);
    const state: State = { ...base, options: { ...base.options, packAs: 'volumes' } };
    const markup = options(state);
    expect(markup).toContain(VOLUMES_HINT_NOT_LOADED);
    expect(markup).toContain('(when loaded)');
  });

  it('shows the standard hint when volumes is selected and a series is loaded', async () => {
    const state = await createMockApi({ scenario: 'volumes', freeze: true }).state();
    expect(state.loaded).toBe(true);
    expect(state.options.packAs).toBe('volumes');
    const markup = options(state);
    expect(markup).toContain(PACK_HINTS.volumes);
    expect(markup).not.toContain('(when loaded)');
  });

  it('shows the chapters hint regardless of loaded state', async () => {
    const base = await createMockApi({ scenario: 'empty', freeze: true }).state();
    const state: State = { ...base, options: { ...base.options, packAs: 'chapters' } };
    const markup = options(state);
    expect(markup).toContain(PACK_HINTS.chapters);
    expect(markup).not.toContain('(when loaded)');
  });

  it('shows the bundle hint regardless of loaded state', async () => {
    const base = await createMockApi({ scenario: 'empty', freeze: true }).state();
    const state: State = { ...base, options: { ...base.options, packAs: 'bundle' } };
    const markup = options(state);
    expect(markup).toContain(PACK_HINTS.bundle);
    expect(markup).not.toContain('(when loaded)');
  });
});

describe('AVIF under Convert to JPEG', () => {
  // the AVIF checkbox's input, as rendered
  function avifInput(markup: string): string {
    const match = markup.match(/<input[^>]*type="checkbox"[^>]*\/>\s*<span>AVIF<\/span>/);
    return match ? match[0] : '';
  }

  it('is checked and locked, with the EPUB hint, when the format is EPUB', async () => {
    const base = await createMockApi({ scenario: 'loaded', freeze: true }).state();
    const state: State = { ...base, options: { ...base.options, format: 'epub', convertImages: 'none' } };
    const markup = options(state);
    expect(avifInput(markup)).toMatch(/checked=""/);
    expect(avifInput(markup)).toMatch(/disabled=""/);
    expect(markup).toContain(AVIF_EPUB_HINT.replace("'", '&#x27;'));
  });

  it('follows the saved value for the other formats', async () => {
    const base = await createMockApi({ scenario: 'loaded', freeze: true }).state();
    const state: State = { ...base, options: { ...base.options, format: 'cbz', convertImages: 'none' } };
    const markup = options(state);
    expect(avifInput(markup)).not.toMatch(/checked=""/);
    expect(avifInput(markup)).not.toMatch(/disabled=""/);
    expect(markup).not.toContain(AVIF_EPUB_HINT.replace("'", '&#x27;'));
  });
});
