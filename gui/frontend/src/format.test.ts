import { describe, expect, it } from 'vitest';
import {
  errorMessage,
  jobFraction,
  jobShortStatus,
  jobStatus,
  plural,
  summaryHeadline,
  summaryLines,
  summarySentence,
} from './format';
import type { Job, Summary } from './types';

const base: Summary = {
  dir: '/out',
  cancelled: false,
  saved: 0,
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

  it('describes cancelled and bundled runs', () => {
    expect(summaryHeadline({ ...base, cancelled: true, saved: 2 })).toBe('Cancelled, 2 saved');
    expect(summaryHeadline({ ...base, cancelled: true, bundled: true })).toBe('Cancelled, no bundle written');
    expect(summaryHeadline({ ...base, bundled: true, bundle: 'a.cbz' })).toBe('Saved a.cbz');
    expect(summaryHeadline({ ...base, bundled: true, error: 'disk full' })).toBe('The bundle could not be written');
  });
});

describe('summaryLines', () => {
  it('lists failures under the outcome', () => {
    const lines = summaryLines({ ...base, saved: 1, failures: ['Ch 4: 404'] });
    expect(lines).toEqual(['Saved 1 chapter to /out.', '1 failed:', '  - Ch 4: 404']);
  });

  it('names the bundle and the chapter count', () => {
    expect(summaryLines({ ...base, bundled: true, bundle: 'b.cbz', saved: 3 })).toEqual([
      'Saved b.cbz to /out (3 chapters).',
    ]);
  });
});

describe('job helpers', () => {
  const job = (over: Partial<Job>): Job => ({
    title: 'Ch 1',
    number: 1,
    total: 24,
    done: 6,
    state: 'downloading',
    reason: '',
    status: '',
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
  it('is the first line of the summary', () => {
    const s = { ...base, saved: 3, failures: ['x: y'] };
    expect(summarySentence(s)).toBe(summaryLines(s)[0]);
    expect(summarySentence({ ...base, cancelled: true })).toBe('Cancelled before anything was saved.');
  });
});

describe('jobShortStatus', () => {
  const job = (over: Partial<Job>): Job => ({
    title: 'Ch 1',
    number: 1,
    total: 24,
    done: 6,
    state: 'downloading',
    reason: '',
    status: '',
    ...over,
  });

  it('shows the phase and percentage while moving', () => {
    expect(jobShortStatus(job({}))).toBe('Downloading · 25%');
    expect(jobShortStatus(job({ state: 'archiving', done: 18 }))).toBe('Packing · 75%');
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
