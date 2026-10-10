import { describe, expect, it } from 'vitest';
import { celebratesOnOpen, celebrating, savedEverything } from './mascot';
import type { Summary } from './types';

const clean = (over: Partial<Summary> = {}): Summary => ({
  dir: '/books',
  cancelled: false,
  saved: 3,
  bundled: false,
  bundle: '',
  skipped: 0,
  failures: [],
  error: '',
  ...over,
});

const idle = (summary: Summary | null) => ({ loading: false, running: false, summary });
const running = { loading: false, running: true, summary: null };
const loading = { loading: true, running: false, summary: null };

describe('savedEverything', () => {
  it('is true for a run that saved every chapter', () => {
    expect(savedEverything(clean())).toBe(true);
  });

  it('is true for a bundle that was written', () => {
    expect(savedEverything(clean({ bundled: true, bundle: 'One Piece.cbz', saved: 0 }))).toBe(true);
  });

  it('is false when the bundle was not written', () => {
    expect(savedEverything(clean({ bundled: true, bundle: '', saved: 0 }))).toBe(false);
  });

  it('is false when a chapter failed', () => {
    expect(savedEverything(clean({ failures: ['Chapter 4: download: page 5 returned 404'] }))).toBe(false);
  });

  it('is false when the run was cancelled', () => {
    expect(savedEverything(clean({ cancelled: true }))).toBe(false);
  });

  it('is false when a bundle could not be written', () => {
    expect(savedEverything(clean({ error: 'disk full' }))).toBe(false);
  });

  it('is true when a chapter was skipped on purpose', () => {
    expect(savedEverything(clean({ skipped: 1 }))).toBe(true);
  });

  it('is false when every chapter was skipped', () => {
    expect(savedEverything(clean({ skipped: 3, saved: 0 }))).toBe(false);
  });

  it('is false when nothing was saved', () => {
    expect(savedEverything(clean({ saved: 0 }))).toBe(false);
  });
});

describe('celebratesOnOpen', () => {
  it('celebrates a window opened on a clean last run', () => {
    expect(celebratesOnOpen(idle(clean()))).toBe(true);
  });

  it('does not celebrate without a summary', () => {
    expect(celebratesOnOpen(idle(null))).toBe(false);
  });

  it('does not celebrate a failed last run', () => {
    expect(celebratesOnOpen(idle(clean({ failures: ['x'] })))).toBe(false);
  });

  it('does not celebrate while a load or a run is going', () => {
    expect(celebratesOnOpen({ ...idle(clean()), loading: true })).toBe(false);
    expect(celebratesOnOpen({ ...idle(clean()), running: true })).toBe(false);
  });
});

describe('celebrating', () => {
  it('starts when a run ends with everything saved', () => {
    expect(celebrating(false, true, idle(clean()))).toBe(true);
  });

  it('stays off when a run ends with a failure', () => {
    expect(celebrating(false, true, idle(clean({ failures: ['x'] })))).toBe(false);
  });

  it('stays off when a run is cancelled', () => {
    expect(celebrating(false, true, idle(clean({ cancelled: true })))).toBe(false);
  });

  it('stops as soon as a download starts', () => {
    expect(celebrating(true, false, running)).toBe(false);
  });

  it('stops as soon as a load starts', () => {
    expect(celebrating(true, false, loading)).toBe(false);
  });

  it('stays on while nothing starts or ends', () => {
    expect(celebrating(true, false, idle(clean()))).toBe(true);
  });

  it('does not come back when a load fails over the old summary', () => {
    const afterLoading = celebrating(true, false, loading);
    expect(celebrating(afterLoading, false, idle(clean()))).toBe(false);
  });
});
