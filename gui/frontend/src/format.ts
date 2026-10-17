// Pure helpers that turn the window's state into the words shown to the user.

import { say, type Lang, type T } from './i18n';
import { percent } from './i18n/format';
import type { Job, Message, Summary } from './types';

export function plural(n: number, one: string, many: string): string {
  return n === 1 ? `1 ${one}` : `${n} ${many}`;
}

/** The short outcome shown next to the buttons */
export function summaryHeadline(s: Summary, t: T): string {
  if (s.cancelled && s.bundled) {
    return t('summary.headline.cancelledNoBundle');
  }
  if (s.cancelled) {
    return t('summary.headline.cancelled', { saved: t('count.saved', { count: s.saved }) });
  }
  if (s.error !== '') {
    return t('summary.headline.bundleFailed');
  }
  if (s.bundled && s.bundle !== '') {
    return t('summary.headline.bundleSaved', { bundle: s.bundle });
  }

  const parts: string[] = [];
  if (s.saved > 0) {
    const saved = t('count.saved', { count: s.saved });
    if (s.volumes > 0 && s.parts) {
      parts.push(t('summary.headline.mergedFromParts', { saved, merged: t('summary.merged', { count: s.volumes }) }));
    } else {
      parts.push(
        s.volumes > 0
          ? t('summary.headline.inVolumes', { saved, volumes: t('count.volume', { count: s.volumes }) })
          : saved,
      );
    }
  }
  if (s.failures.length > 0) {
    parts.push(t('count.failed', { count: s.failures.length }));
  }
  if (s.skipped > 0) {
    parts.push(t('count.skipped', { count: s.skipped }));
  }

  return parts.length === 0 ? t('summary.headline.nothingSaved') : parts.join(', ');
}

/** The one sentence that says where a finished run's chapters went */
export function summarySentence(s: Summary, t: T): string {
  if (s.cancelled && s.bundled) {
    return t('summary.sentence.cancelledNoBundle');
  }
  if (s.cancelled && s.saved === 0) {
    return t('summary.sentence.cancelledBeforeSaving');
  }
  if (s.cancelled) {
    return t('summary.sentence.savedBefore', { count: s.saved, dir: s.dir });
  }
  if (s.error !== '') {
    return t('summary.sentence.bundleFailed');
  }
  if (s.bundled && s.bundle !== '') {
    return t('summary.sentence.bundleSaved', {
      bundle: s.bundle,
      dir: s.dir,
      chapters: t('count.chapter', { count: s.saved }),
    });
  }
  if (s.saved > 0 && s.volumes > 0 && s.parts) {
    return t('summary.sentence.mergedFromParts', {
      count: s.saved,
      dir: s.dir,
      merged: t('summary.merged', { count: s.volumes }),
    });
  }
  if (s.saved > 0 && s.volumes > 0) {
    return t('summary.sentence.volumeFiles', {
      count: s.saved,
      dir: s.dir,
      files: t('count.volumeFile', { count: s.volumes }),
    });
  }
  if (s.saved > 0) {
    return t('summary.sentence.saved', { count: s.saved, dir: s.dir });
  }

  return t('summary.sentence.nothingSaved');
}

/** The status text of a progress row */
export function jobStatus(j: Job, t: T): string {
  switch (j.state) {
    case 'fetching':
      return j.status !== '' ? say(t, j.statusMsg, j.status) : t('job.status.fetching');
    case 'downloading':
      return j.status !== '' ? say(t, j.statusMsg, j.status) : t('job.status.downloading');
    case 'archiving':
      return j.status !== '' ? say(t, j.statusMsg, j.status) : t('job.status.archiving');
    case 'done':
      return t('job.status.done');
    case 'failed':
      return t('job.status.failed', { reason: say(t, j.reasonMsg, j.reason) });
    case 'skipped':
      return t('job.status.skipped', { reason: say(t, j.reasonMsg, j.reason) });
  }
}

/** The short status at the end of a progress row: phase and percentage while it moves, the outcome after */
export function jobShortStatus(j: Job, t: T, lang: Lang): string {
  switch (j.state) {
    case 'fetching':
      return t('job.short.fetching');
    case 'downloading':
    case 'archiving': {
      const phase =
        j.status !== ''
          ? say(t, j.statusMsg, j.status).replace(/\.+$/, '')
          : j.state === 'archiving'
            ? t('job.short.packing')
            : t('job.short.downloading');
      return t('job.short.phase', { phase, percent: percent(jobFraction(j), lang) });
    }
    case 'done':
      return t('job.short.done');
    case 'failed':
      return t('job.short.failed');
    case 'skipped':
      return t('job.short.skipped');
  }
}

/** How far along a row is, from 0 to 1 */
export function jobFraction(j: Job): number {
  if (j.state === 'done') {
    return 1;
  }
  if (j.total <= 0) {
    return 0;
  }

  return Math.min(1, Math.max(0, j.done / j.total));
}

/** Text for a thrown value from a bound call, which Wails rejects with a string */
export function errorText(e: unknown): string {
  if (typeof e === 'string') {
    return e;
  }
  if (e instanceof Error) {
    return e.message;
  }

  return String(e);
}

/**
 * What to show for a thrown value from a bound call. The Go side sends an error
 * it wrote as its message's JSON, which is translated here; other text is shown as it is.
 */
export function errorMessage(e: unknown, t: T): string {
  const text = errorText(e);
  const msg = parseMessage(text);

  return msg ? say(t, msg, msg.text) : text;
}

function parseMessage(text: string): Message | undefined {
  if (!text.startsWith('{')) {
    return undefined;
  }
  try {
    const parsed: unknown = JSON.parse(text);
    if (typeof parsed === 'object' && parsed !== null && typeof (parsed as { key?: unknown }).key === 'string') {
      return parsed as Message;
    }
  } catch {
    // not JSON: the text is shown as it is
  }

  return undefined;
}

/** A chapter number as the site would print it: 12, 12.5, never 12.000001 */
export function formatNumber(n: number): string {
  if (Number.isInteger(n)) {
    return String(n);
  }

  return String(Math.round(n * 100) / 100);
}
