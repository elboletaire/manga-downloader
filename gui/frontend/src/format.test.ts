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
import type { Job, Summary } from './types';

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
    expect(summaryHeadline({ ...base, saved: 9, failures: ['x'], skipped: 1 })).toBe('9 saved, 1 failed, 1 skipped');
    expect(summaryHeadline(base)).toBe('Nothing was saved');
  });

  it('describes a volume run', () => {
    expect(summaryHeadline({ ...base, saved: 12, volumes: 2 })).toBe('12 saved in 2 volumes');
    expect(summarySentence({ ...base, saved: 12, volumes: 1 })).toBe('Saved 12 chapters to /out, with 1 volume file.');
    expect(summaryHeadline({ ...base, saved: 8, volumes: 3, parts: true })).toBe('8 saved, 3 chapters merged from parts');
    expect(summarySentence({ ...base, saved: 8, volumes: 3, parts: true })).toBe(
      'Saved 8 chapters to /out, with 3 chapters merged from parts.',
    );
  });

  it('describes cancelled and bundled runs', () => {
    expect(summaryHeadline({ ...base, cancelled: true, saved: 2 })).toBe('Cancelled, 2 saved');
    expect(summaryHeadline({ ...base, cancelled: true, bundled: true })).toBe('Cancelled, no bundle written');
    expect(summaryHeadline({ ...base, bundled: true, bundle: 'a.cbz' })).toBe('Saved a.cbz');
    expect(summaryHeadline({ ...base, bundled: true, error: 'disk full' })).toBe('The bundle could not be written');
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
    expect(jobStatus(job({ status: 'Creating bundle' }))).toBe('Creating bundle');
    expect(jobStatus(job({ state: 'archiving' }))).toBe('archiving');
    expect(jobStatus(job({ state: 'failed', reason: 'packing: x' }))).toBe('failed: packing: x');
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
    expect(summarySentence({ ...base, cancelled: true })).toBe('Cancelled before anything was saved.');
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
    expect(jobShortStatus(job({}))).toBe('Downloading · 25%');
    expect(jobShortStatus(job({ state: 'archiving', done: 18 }))).toBe('Packing · 75%');
  });

  it('says a row is fetching its pages without a percentage', () => {
    expect(jobShortStatus(job({ state: 'fetching', total: 0, done: 0 }))).toBe('Fetching pages\u2026');
  });

  it("uses the bundle's own phase without its trailing dots", () => {
    expect(jobShortStatus(job({ status: 'Creating the bundle...', done: 12 }))).toBe('Creating the bundle · 50%');
  });

  it('names the outcome once finished', () => {
    expect(jobShortStatus(job({ state: 'done' }))).toBe('Saved');
    expect(jobShortStatus(job({ state: 'failed', reason: '404' }))).toBe('Failed');
    expect(jobShortStatus(job({ state: 'skipped' }))).toBe('Skipped');
  });
});

describe('formatNumber', () => {
  it('prints whole and fractional chapter numbers cleanly', () => {
    expect(formatNumber(12)).toBe('12');
    expect(formatNumber(12.5)).toBe('12.5');
    expect(formatNumber(1.0100000001)).toBe('1.01');
  });
});
