import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { createMockApi, templateWarning } from './mock';
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

describe('Filename template', () => {
  it('offers the reset button only when the template differs from the default', async () => {
    const base = await createMockApi({ scenario: 'empty', freeze: true }).state();
    expect(base.options.filenameTemplate).toBe(base.filenameTemplateDefault);
    expect(options(base)).not.toContain('Use default template');
    // the wrapper stays when the button is hidden, so the input isn't remounted
    expect(options(base)).toContain('class="input-wrap"');

    const custom: State = { ...base, options: { ...base.options, filenameTemplate: '{{.Series}} {{.Number}}' } };
    expect(options(custom)).toContain('aria-label="Use default template"');

    const blank: State = { ...base, options: { ...base.options, filenameTemplate: '' } };
    expect(options(blank)).toContain('aria-label="Use default template"');
  });

  it('draws the reset button inside the box and disables it while locked', async () => {
    const base = await createMockApi({ scenario: 'empty', freeze: true }).state();
    const custom: State = { ...base, options: { ...base.options, filenameTemplate: '{{.Series}}' } };
    expect(options(custom)).toMatch(/<div class="input-wrap">.*<div class="input-action"><button[^>]*aria-label="Use default template"/);

    const locked: State = { ...custom, running: true };
    expect(options(locked)).toMatch(/<button[^>]*aria-label="Use default template"[^>]*disabled=""/);
  });

  it('shows the default as the placeholder of an empty box', async () => {
    const base = await createMockApi({ scenario: 'empty', freeze: true }).state();
    const blank: State = { ...base, options: { ...base.options, filenameTemplate: '' } };
    expect(options(blank)).toContain(`placeholder="${base.filenameTemplateDefault.replace(/"/g, '&quot;')}"`);
  });

  it('warns when the template names no number or title', async () => {
    const base = await createMockApi({ scenario: 'empty', freeze: true }).state();
    expect(options(base)).not.toContain('files only differ');

    const warned: State = { ...base, options: { ...base.options, filenameTemplate: '{{.Series}}' }, filenameTemplateWarning: 'files only differ by a version suffix.' };
    expect(options(warned)).toContain('files only differ');
  });

  it('puts the default back through the mock and clears the warning', async () => {
    const api = createMockApi({ scenario: 'empty', freeze: true });
    const base = await api.state();
    await api.setOptions({ ...base.options, filenameTemplate: 'fixed' });
    expect((await api.state()).filenameTemplateWarning).not.toBe('');
    await api.setOptions({ ...base.options, filenameTemplate: base.filenameTemplateDefault });
    expect((await api.state()).filenameTemplateWarning).toBe('');
  });
});

describe('templateWarning (mock)', () => {
  it('matches the Go cases', () => {
    for (const ok of ['', '  ', '{{ .Number }}', '{{- .Number}}', '{{printf "%03s" .Number}}', '{{pad .Number 4}}', '{{.Series}} {{.Volume}}']) {
      expect(templateWarning(ok), ok).toBe('');
    }
    expect(templateWarning('{{.Series}}')).not.toBe('');
  });
});
