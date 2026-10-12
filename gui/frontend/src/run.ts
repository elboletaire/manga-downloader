// Pure helpers about the run as a whole: whether Download can start, how far
// along it is, and how it ended. format.ts has the per-job and summary wording.

import { jobFraction, plural } from './format';
import type { Job, JobState, State, Summary } from './types';

/** The states a job ends in */
export function isFinished(j: Job): boolean {
  return j.state === 'done' || j.state === 'failed' || j.state === 'skipped';
}

export type Tally = Record<JobState, number>;

export function tally(jobs: Job[]): Tally {
  const t: Tally = { downloading: 0, archiving: 0, done: 0, failed: 0, skipped: 0 };
  for (const j of jobs) {
    t[j.state]++;
  }

  return t;
}

export interface Overall {
  /** Rows (chapters, volumes or the one bundle) that reached an end state */
  finished: number;
  /** What the run covers: its chapters, its volumes plus the chapters outside them, or 1 for a bundle */
  total: number;
  /** Chapters the backend hasn't started yet; they have no job until they do */
  queued: number;
  /** 0 to 1, page-weighted inside each job */
  fraction: number;
}

/**
 * The run's overall progress. Jobs are only created as chapters start, so the
 * run's chapter count is the denominator, not the job count.
 */
export function overallProgress(state: Pick<State, 'jobs' | 'run'>): Overall {
  const { jobs, run } = state;
  if (run.bundle) {
    const job = jobs[0];
    const finished = job !== undefined && isFinished(job) ? 1 : 0;
    return { finished, total: 1, queued: 0, fraction: job ? jobFraction(job) : 0 };
  }

  const total = Math.max(run.rows > 0 ? run.rows : run.chapters, jobs.length);
  if (total === 0) {
    return { finished: 0, total: 0, queued: 0, fraction: 0 };
  }
  let sum = 0;
  let finished = 0;
  for (const j of jobs) {
    if (isFinished(j)) {
      finished++;
      sum += 1;
    } else {
      sum += jobFraction(j);
    }
  }

  return { finished, total, queued: total - jobs.length, fraction: Math.min(1, sum / total) };
}

/**
 * Why Download can't start right now, or "" when it can. These are the
 * refusals the backend would answer with, shown before the click instead of
 * after it. The backend stays the authority: its own refusal still shows.
 */
export function downloadBlocker(state: State): string {
  if (state.loading) {
    return 'Wait for the series to finish loading.';
  }
  if (state.running) {
    return '';
  }
  if (!state.loaded) {
    return 'Load a series to pick its chapters.';
  }
  if (state.stale) {
    return 'The filters changed: reload the series first.';
  }
  if (state.selectedCount === 0) {
    return 'Select at least one chapter.';
  }
  if (state.options.packAs === 'volumes' && isSiteVolumes(state.volumes.text) && state.volumes.site === '') {
    return 'The site lists no volumes: enter them as chapter ranges, e.g. 1-7; 8-16.';
  }

  return '';
}

/** Whether the Volumes box stands for the site's own volumes */
function isSiteVolumes(text: string): boolean {
  const t = text.trim().toLowerCase();
  return t === '' || t === 'auto';
}

/** The Download button's label */
export function downloadLabel(state: Pick<State, 'selectedCount' | 'options'>): string {
  const n = state.selectedCount;
  if (n === 0) {
    return 'Download';
  }
  if (state.options.packAs === 'bundle' && n > 1) {
    return `Download ${n} chapters as one file`;
  }
  if (state.options.packAs === 'volumes') {
    return `Download ${plural(n, 'chapter', 'chapters')} by volume`;
  }

  return `Download ${plural(n, 'chapter', 'chapters')}`;
}

export type Tone = 'success' | 'warning' | 'danger' | 'neutral';

/** The colour a finished run is shown in */
export function summaryTone(s: Summary): Tone {
  if (s.error !== '') {
    return 'danger';
  }
  if (s.cancelled) {
    return 'neutral';
  }
  if (s.failures.length > 0) {
    return s.saved > 0 || s.volumes > 0 || (s.bundled && s.bundle !== '') ? 'warning' : 'danger';
  }
  if (s.saved === 0 && s.volumes === 0 && !(s.bundled && s.bundle !== '')) {
    return 'neutral';
  }

  return 'success';
}

/**
 * Splits a failure line ("Chapter 4: download: page 5 returned 404") into
 * what failed and why, at the first ": ". A line without one is all reason.
 */
export function splitFailure(line: string): { what: string; why: string } {
  const at = line.indexOf(': ');
  if (at <= 0) {
    return { what: '', why: line };
  }

  return { what: line.slice(0, at), why: line.slice(at + 2) };
}

/** The host of a URL without "www.", or "" when it doesn't parse */
export function hostOf(url: string): string {
  try {
    return new URL(url.trim()).hostname.replace(/^www\./, '');
  } catch {
    return '';
  }
}

/** A chapter number as the site would print it: 12, 12.5, never 12.000001 */
export function formatNumber(n: number): string {
  if (Number.isInteger(n)) {
    return String(n);
  }

  return String(Math.round(n * 100) / 100);
}
