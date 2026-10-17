import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { I18nProvider } from './i18n/react';
import { DownloadDock } from './DownloadDock';
import { createMockApi } from './mock';
import type { Chapter, State } from './types';

async function render(lang: 'en' | 'es' | 'ca', over: Partial<State> = {}, error = ''): Promise<string> {
  const state = { ...(await createMockApi('loaded').state()), ...over };
  const chapters: Chapter[] = [];
  return renderToStaticMarkup(
    createElement(
      I18nProvider,
      { pref: lang, persist: async () => {} },
      createElement(DownloadDock, { state, error, chapters, selected: [], onStart: () => {}, onCancel: () => {} }),
    ),
  );
}

describe('DownloadDock', () => {
  it('shows the idle dock in the chosen language', async () => {
    const es = await render('es');
    expect(es).toContain('Descargar ');
    expect(es).toContain('Archivos CBZ en');
    const ca = await render('ca');
    expect(ca).toContain('Descarrega ');
    expect(ca).toContain('Fitxers CBZ en');
  });

  it('translates the running progress and the cancelling state', async () => {
    const running = { running: true, run: { chapters: 4, bundle: false, volumes: false, rows: 0 }, jobs: [] };
    const es = await render('es', { ...running, cancelling: true });
    expect(es).toContain('Terminando los capítulos en curso');
    expect(es).toContain('Cancelando…');
    const ca = await render('ca', running);
    expect(ca).toContain('0 de 4 capítols fets');
    expect(ca).toContain('Cancel·la');
  });

  it('translates the refused download', async () => {
    expect(await render('es', {}, 'disco lleno')).toContain('No se puede descargar: disco lleno');
  });
});
