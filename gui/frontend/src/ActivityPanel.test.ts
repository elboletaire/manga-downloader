import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { ActivityPanel } from './ActivityPanel';
import type { LangPref } from './i18n';
import { I18nProvider } from './i18n/react';
import type { Job, State, Summary } from './types';

const summary: Summary = {
  dir: '/out',
  cancelled: false,
  saved: 3,
  volumes: 0,
  parts: false,
  bundled: false,
  bundle: '',
  skipped: 0,
  failures: [],
  error: '',
};

const job: Job = {
  title: 'Ch 1',
  tags: '',
  number: 1,
  total: 24,
  done: 6,
  state: 'downloading',
  reason: '',
  status: '',
  volume: false,
};

function render(state: Partial<State>, pref: LangPref = '') {
  return renderToStaticMarkup(
    createElement(
      I18nProvider,
      { pref, persist: async () => {}, systemLanguages: ['en-US'] },
      createElement(ActivityPanel, { state: state as State, onOpenFolder: () => undefined }),
    ),
  );
}

describe('ActivityPanel', () => {
  it('shows the empty state in English by default', () => {
    const html = render({ running: false, summary: null, jobs: [] });
    expect(html).toContain('Nothing downloaded yet.');
  });

  it('shows the empty state in Spanish', () => {
    const html = render({ running: false, summary: null, jobs: [] }, 'es');
    expect(html).toContain('Todavía no se ha descargado nada.');
    expect(html).not.toContain('Nothing downloaded');
  });

  it('labels and words the last summary in Catalan', () => {
    const html = render({ running: false, summary, jobs: [] }, 'ca');
    expect(html).toContain('aria-label="Última descàrrega"');
    expect(html).toContain('<h3>3 desats</h3>');
  });

  it('shows a volume row and its reason in Spanish', () => {
    const volume: Job = {
      ...job,
      title: 'Vol. 01 · chapters 1–7',
      titleMsg: {
        key: 'job.title.volume',
        args: { volume: 'Vol. 01', first: '1', last: '7' },
        text: 'Vol. 01 · chapters 1–7',
      },
      state: 'failed',
      reason: 'download: x',
      reasonMsg: { key: 'job.reason.download', args: { reason: 'x' }, text: 'download: x' },
    };
    const html = render({ running: true, summary: null, jobs: [volume], run: { chapters: 7, bundle: false, volumes: true, rows: 1 } }, 'es');
    expect(html).toContain('Vol. 01 · capítulos 1–7');
    expect(html).toContain('descarga: x');
  });

  it('titles the running chapters and their progress in Spanish', () => {
    const html = render(
      { running: true, summary: null, jobs: [job], run: { chapters: 1, bundle: false, volumes: false, rows: 1 } },
      'es',
    );
    expect(html).toContain('Esta descarga');
    expect(html).toContain('aria-label="Progreso de Ch 1"');
  });
});
