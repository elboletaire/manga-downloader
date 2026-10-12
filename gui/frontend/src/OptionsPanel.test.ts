import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { createMockApi } from './mock';
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
