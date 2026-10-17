import { describe, expect, it } from 'vitest';
import {
  errorMessage,
  formatNumber,
  jobFraction,
  jobShortStatus,
  jobStatus,
  plural,
  summaryHeadline,
  summarySentence,
} from './format';
import { createT } from './i18n';
import type { Job, Summary } from './types';

const en = createT('en');
const es = createT('es');
const ca = createT('ca');

const base: Summary = {
  dir: '/out',
  cancelled: false,
  saved: 0,
  volumes: 0,
  parts: false,
  bundled: false,
  bundle: '',
  skipped: 0,
  failures: [],
  error: '',
};

describe('plural', () => {
  it('uses the singular only for one', () => {
    expect(plural(1, 'chapter', 'chapters')).toBe('1 chapter');
    expect(plural(0, 'chapter', 'chapters')).toBe('0 chapters');
    expect(plural(2, 'chapter', 'chapters')).toBe('2 chapters');
  });
});

describe('summaryHeadline', () => {
  it('describes a plain run', () => {
    expect(summaryHeadline({ ...base, saved: 9, failures: ['x'], skipped: 1 }, en)).toBe(
      '9 saved, 1 failed, 1 skipped',
    );
    expect(summaryHeadline(base, en)).toBe('Nothing was saved');
  });

  it('describes a volume run', () => {
    expect(summaryHeadline({ ...base, saved: 12, volumes: 2 }, en)).toBe('12 saved in 2 volumes');
    expect(summarySentence({ ...base, saved: 12, volumes: 1 }, en)).toBe(
      'Saved 12 chapters to /out, with 1 volume file.',
    );
    expect(summaryHeadline({ ...base, saved: 8, volumes: 3, parts: true }, en)).toBe(
      '8 saved, 3 chapters merged from parts',
    );
    expect(summarySentence({ ...base, saved: 8, volumes: 3, parts: true }, en)).toBe(
      'Saved 8 chapters to /out, with 3 chapters merged from parts.',
    );
  });

  it('describes cancelled and bundled runs', () => {
    expect(summaryHeadline({ ...base, cancelled: true, saved: 2 }, en)).toBe('Cancelled, 2 saved');
    expect(summaryHeadline({ ...base, cancelled: true, bundled: true }, en)).toBe('Cancelled, no bundle written');
    expect(summaryHeadline({ ...base, bundled: true, bundle: 'a.cbz' }, en)).toBe('Saved a.cbz');
    expect(summaryHeadline({ ...base, bundled: true, error: 'disk full' }, en)).toBe('The bundle could not be written');
  });
});

describe('job helpers', () => {
  const job = (over: Partial<Job>): Job => ({
    title: 'Ch 1',
    tags: '',
    number: 1,
    total: 24,
    done: 6,
    state: 'downloading',
    reason: '',
    status: '',
    volume: false,
    ...over,
  });

  it('uses the bundle phase when there is one', () => {
    expect(jobStatus(job({ status: 'Creating bundle' }), en)).toBe('Creating bundle');
    expect(jobStatus(job({ state: 'archiving' }), en)).toBe('archiving');
    expect(jobStatus(job({ state: 'failed', reason: 'packing: x' }), en)).toBe('failed: packing: x');
  });

  it('translates the phase and reason the GUI wrote, and keeps the English without them', () => {
    const phase = { key: 'job.phase.downloadingLabel', args: { label: 'Ch 5' }, text: 'Downloading Ch 5' };
    expect(jobStatus(job({ status: 'Downloading Ch 5', statusMsg: phase }), es)).toBe('Descargando Ch 5');
    expect(jobStatus(job({ status: 'Downloading Ch 5' }), es)).toBe('Downloading Ch 5');
    const reason = { key: 'job.reason.download', args: { reason: 'x' }, text: 'download: x' };
    expect(jobStatus(job({ state: 'failed', reason: 'download: x', reasonMsg: reason }), es)).toBe('fallido: descarga: x');
  });

  it('fills to 1 when done and clamps the fraction', () => {
    expect(jobFraction(job({ done: 6 }))).toBe(0.25);
    expect(jobFraction(job({ state: 'done', done: 3 }))).toBe(1);
    expect(jobFraction(job({ total: 0 }))).toBe(0);
    expect(jobFraction(job({ done: 99 }))).toBe(1);
  });
});

describe('errorMessage', () => {
  it('reads a Wails rejection, which is a plain string', () => {
    expect(errorMessage('select at least one chapter')).toBe('select at least one chapter');
    expect(errorMessage(new Error('boom'))).toBe('boom');
    expect(errorMessage(42)).toBe('42');
  });
});

describe('summarySentence', () => {
  it('says a cancelled run saved nothing', () => {
    expect(summarySentence({ ...base, cancelled: true }, en)).toBe('Cancelled before anything was saved.');
  });
});

describe('jobShortStatus', () => {
  const job = (over: Partial<Job>): Job => ({
    title: 'Ch 1',
    tags: '',
    number: 1,
    total: 24,
    done: 6,
    state: 'downloading',
    reason: '',
    status: '',
    volume: false,
    ...over,
  });

  it('shows the phase and percentage while moving', () => {
    expect(jobShortStatus(job({}), en, 'en')).toBe('Downloading · 25%');
    expect(jobShortStatus(job({ state: 'archiving', done: 18 }), en, 'en')).toBe('Packing · 75%');
  });

  it('says a row is fetching its pages without a percentage', () => {
    expect(jobShortStatus(job({ state: 'fetching', total: 0, done: 0 }), en, 'en')).toBe('Fetching pages\u2026');
  });

  it("uses the bundle's own phase without its trailing dots", () => {
    expect(jobShortStatus(job({ status: 'Creating the bundle...', done: 12 }), en, 'en')).toBe('Creating the bundle · 50%');
  });

  it('names the outcome once finished', () => {
    expect(jobShortStatus(job({ state: 'done' }), en, 'en')).toBe('Saved');
    expect(jobShortStatus(job({ state: 'failed', reason: '404' }), en, 'en')).toBe('Failed');
    expect(jobShortStatus(job({ state: 'skipped' }), en, 'en')).toBe('Skipped');
  });
});

describe('formatNumber', () => {
  it('prints whole and fractional chapter numbers cleanly', () => {
    expect(formatNumber(12)).toBe('12');
    expect(formatNumber(12.5)).toBe('12.5');
    expect(formatNumber(1.0100000001)).toBe('1.01');
  });
});

describe('translated helpers', () => {
  it('reads the summary in Spanish and Catalan', () => {
    expect(summaryHeadline({ ...base, saved: 9, failures: ['x'], skipped: 1 }, es)).toBe(
      '9 guardados, 1 fallido, 1 omitido',
    );
    expect(summarySentence({ ...base, saved: 12, volumes: 1 }, es)).toBe(
      'Se han guardado 12 capítulos en /out, con 1 archivo de volumen.',
    );
    expect(summaryHeadline({ ...base, cancelled: true, saved: 2 }, ca)).toBe('Cancel·lat, 2 desats');
  });

  it('agrees the Spanish sentence with a count of one and of two', () => {
    expect(summarySentence({ ...base, saved: 1 }, es)).toBe('Se ha guardado 1 capítulo en /out.');
    expect(summarySentence({ ...base, saved: 2 }, es)).toBe('Se han guardado 2 capítulos en /out.');
    expect(summarySentence({ ...base, saved: 1, volumes: 1, parts: true }, es)).toBe(
      'Se ha guardado 1 capítulo en /out, con 1 capítulo unido desde partes.',
    );
    expect(summarySentence({ ...base, saved: 2, volumes: 2, parts: true }, es)).toBe(
      'Se han guardado 2 capítulos en /out, con 2 capítulos unidos desde partes.',
    );
    expect(summarySentence({ ...base, cancelled: true, saved: 1 }, es)).toBe(
      'Cancelado. 1 capítulo guardado antes de detenerse, en /out.',
    );
  });

  it('formats the percentage in the language', () => {
    const job: Job = {
      title: 'Ch 1',
      tags: '',
      number: 1,
      total: 24,
      done: 18,
      state: 'archiving',
      reason: '',
      status: '',
      volume: false,
    };
    expect(jobShortStatus(job, es, 'es')).toBe('Empaquetando · 75\u00a0%');
    expect(jobShortStatus(job, en, 'en')).toBe('Packing · 75%');
  });

  it('reads the job status in Catalan', () => {
    const job: Job = {
      title: 'Ch 1',
      tags: '',
      number: 1,
      total: 24,
      done: 18,
      state: 'archiving',
      reason: '',
      status: '',
      volume: false,
    };
    expect(jobShortStatus(job, ca, 'ca')).toBe('Empaquetant · 75\u00a0%');
    expect(jobStatus({ ...job, state: 'failed', reason: '404' }, ca)).toBe('fallit: 404');
  });
});
