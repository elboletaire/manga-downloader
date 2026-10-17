// Pure helpers about the run as a whole: whether Download can start, how far
// along it is, and how it ended. format.ts has the per-job and summary wording.

import { jobFraction } from './format';
import type { T } from './i18n';
import type { Chapter, Job, JobState, State, Summary } from './types';
import { groupChapters, groupParts, isSiteVolumes, parseVolumes, selectedIn, type Grouping } from './volumes';

/** The states a job ends in */
export function isFinished(j: Job): boolean {
  return j.state === 'done' || j.state === 'failed' || j.state === 'skipped';
}

export type Tally = Record<JobState, number>;

export function tally(jobs: Job[]): Tally {
  const t: Tally = { fetching: 0, downloading: 0, archiving: 0, done: 0, failed: 0, skipped: 0 };
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
export function downloadBlocker(state: State, t: T): string {
  if (state.loading) {
    return t('download.blocker.loading');
  }
  if (state.running) {
    return '';
  }
  if (!state.loaded) {
    return t('download.blocker.notLoaded');
  }
  if (state.stale) {
    return t('download.blocker.stale');
  }
  if (state.selectedCount === 0) {
    return t('download.blocker.noChapters');
  }
  if (state.options.packAs === 'volumes' && isSiteVolumes(state.volumes.text) && state.volumes.listed === 0) {
    return t('download.blocker.noVolumes', { example: '1-7/8-16' });
  }

  return '';
}

/** The Download button's label; chapters and selected are the loaded list and its selected indices */
export function downloadLabel(
  state: Pick<State, 'selectedCount' | 'options' | 'volumes'>,
  chapters: Chapter[],
  selected: number[],
  t: T,
): string {
  const n = state.selectedCount;
  if (n === 0) {
    return t('download.label.download');
  }
  if (state.options.packAs === 'bundle' && n > 1) {
    return t('download.label.bundle', { chapters: t('count.chapter', { count: n }) });
  }
  if (state.options.packAs === 'volumes') {
    const label = volumesLabel(state.volumes.text, chapters, selected, t);
    if (label !== '') {
      return label;
    }
  }
  if (state.options.packAs === 'parts') {
    return partsLabel(chapters, selected, t);
  }

  return t('download.label.chapters', { chapters: t('count.chapter', { count: n }) });
}

/**
 * The label of a volumes run: the volumes with a selected chapter, and the
 * selected chapters outside them. "" when the Volumes box doesn't parse or no
 * volume has a selected chapter, so the caller falls back to the chapter count.
 */
function volumesLabel(text: string, chapters: Chapter[], selected: number[], t: T): string {
  let grouping: Grouping;
  try {
    grouping = groupChapters(chapters, parseVolumes(text));
  } catch {
    return '';
  }
  const chosen = new Set(selected);
  const volumes = grouping.volumes.map((v) => selectedIn(v.chapters, chosen)).filter((k) => k > 0);
  if (volumes.length === 0) {
    return '';
  }
  const inVolumes = volumes.reduce((sum, k) => sum + k, 0);
  const loose = selectedIn(grouping.loose, chosen);
  const named = t('count.volume', { count: volumes.length });
  if (loose > 0) {
    return t('download.label.volumesAndChapters', { volumes: named, chapters: t('count.chapter', { count: loose }) });
  }

  return t('download.label.volumes', { volumes: named, chapters: t('count.chapter', { count: inVolumes }) });
}

/**
 * The label of a parts run: one file per chapter, so it counts the chapters
 * the selected parts make, and says so only when some of them merge.
 */
function partsLabel(chapters: Chapter[], selected: number[], t: T): string {
  const chosen = new Set(selected);
  let files = 0;
  let merged = false;
  for (const g of groupParts(chapters)) {
    const k = selectedIn(g.chapters, chosen);
    files += k > 0 ? 1 : 0;
    merged ||= k > 1;
  }
  const noun = t('count.chapter', { count: files });

  return merged
    ? t('download.label.partsMerged', { chapters: noun })
    : t('download.label.chapters', { chapters: noun });
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

/** The host of a URL without "www.", or "" when it doesn't parse */
export function hostOf(url: string): string {
  try {
    return new URL(url.trim()).hostname.replace(/^www\./, '');
  } catch {
    return '';
  }
}
