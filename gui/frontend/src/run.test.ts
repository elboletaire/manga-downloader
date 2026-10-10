import { describe, expect, it } from 'vitest';
import { createMockApi } from './mock';
import {
  downloadBlocker,
  downloadLabel,
  formatNumber,
  hostOf,
  overallProgress,
  splitFailure,
  summaryTone,
  tally,
} from './run';
import type { Job, State, Summary } from './types';

const job = (over: Partial<Job>): Job => ({
  title: 'Ch',
  number: 1,
  total: 20,
  done: 0,
  state: 'downloading',
  reason: '',
  status: '',
  ...over,
});

const summary: Summary = {
  dir: '/out',
  cancelled: false,
  saved: 0,
  bundled: false,
  bundle: '',
  skipped: 0,
  failures: [],
  error: '',
};

async function loadedState(over: Partial<State> = {}): Promise<State> {
  return { ...(await createMockApi('loaded').state()), ...over };
}

describe('overallProgress', () => {
  it('counts chapters that have no job yet as queued', () => {
    const o = overallProgress({
      run: { chapters: 4, bundle: false },
      jobs: [job({ state: 'done', done: 20 }), job({ done: 10 })],
    });
    expect(o).toEqual({ finished: 1, total: 4, queued: 2, fraction: 1.5 / 4 });
  });

  it('counts a failed or skipped chapter as finished', () => {
    const o = overallProgress({
      run: { chapters: 2, bundle: false },
      jobs: [job({ state: 'failed', done: 3 }), job({ state: 'skipped', total: 0 })],
    });
    expect(o.finished).toBe(2);
    expect(o.fraction).toBe(1);
  });

  it("follows the bundle's single job", () => {
    const o = overallProgress({ run: { chapters: 9, bundle: true }, jobs: [job({ done: 5 })] });
    expect(o).toEqual({ finished: 0, total: 1, queued: 0, fraction: 0.25 });
  });

  it('is empty before any run', () => {
    expect(overallProgress({ run: { chapters: 0, bundle: false }, jobs: [] }).fraction).toBe(0);
  });
});

describe('tally', () => {
  it('counts jobs per state', () => {
    const t = tally([job({}), job({ state: 'done' }), job({ state: 'done' })]);
    expect(t.done).toBe(2);
    expect(t.downloading).toBe(1);
    expect(t.failed).toBe(0);
  });
});

describe('downloadBlocker', () => {
  it('lets a loaded series with a selection download', async () => {
    expect(downloadBlocker(await loadedState())).toBe('');
  });

  it('explains each refusal before the click', async () => {
    expect(downloadBlocker(await createMockApi('empty').state())).toMatch(/Load a series/);
    expect(downloadBlocker(await loadedState({ loading: true }))).toMatch(/loading/);
    expect(downloadBlocker(await loadedState({ stale: true }))).toMatch(/reload/);
    expect(downloadBlocker(await loadedState({ selectedCount: 0 }))).toMatch(/Select at least one/);
  });
});

describe('downloadLabel', () => {
  it('names the count, and the bundle when there is one', async () => {
    const s = await loadedState();
    expect(downloadLabel({ ...s, selectedCount: 0 })).toBe('Download');
    expect(downloadLabel({ ...s, selectedCount: 1 })).toBe('Download 1 chapter');
    expect(downloadLabel({ ...s, selectedCount: 5 })).toBe('Download 5 chapters');
    expect(downloadLabel({ ...s, selectedCount: 5, options: { ...s.options, bundle: true } })).toBe(
      'Download 5 chapters as one file',
    );
  });
});

describe('summaryTone', () => {
  it('maps outcomes to colours', () => {
    expect(summaryTone({ ...summary, saved: 3 })).toBe('success');
    expect(summaryTone({ ...summary, saved: 3, failures: ['a'] })).toBe('warning');
    expect(summaryTone({ ...summary, failures: ['a'] })).toBe('danger');
    expect(summaryTone({ ...summary, bundled: true, error: 'disk full' })).toBe('danger');
    expect(summaryTone({ ...summary, cancelled: true, saved: 2 })).toBe('neutral');
    expect(summaryTone(summary)).toBe('neutral');
    expect(summaryTone({ ...summary, bundled: true, bundle: 'a.cbz', saved: 3 })).toBe('success');
  });
});

describe('splitFailure', () => {
  it('splits at the first colon', () => {
    expect(splitFailure('Chapter 4: download: page 5 returned 404')).toEqual({
      what: 'Chapter 4',
      why: 'download: page 5 returned 404',
    });
    expect(splitFailure('no colon here')).toEqual({ what: '', why: 'no colon here' });
  });
});

describe('hostOf', () => {
  it('drops www and survives junk', () => {
    expect(hostOf('https://www.example.com/series/a')).toBe('example.com');
    expect(hostOf(' https://sub.example.org/x ')).toBe('sub.example.org');
    expect(hostOf('not a url')).toBe('');
  });
});

describe('formatNumber', () => {
  it('prints whole and fractional chapter numbers cleanly', () => {
    expect(formatNumber(12)).toBe('12');
    expect(formatNumber(12.5)).toBe('12.5');
    expect(formatNumber(1.0100000001)).toBe('1.01');
  });
});
