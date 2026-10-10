// Pure helpers that turn the window's state into the words shown to the user.

import type { Job, Summary } from './types';

export function plural(n: number, one: string, many: string): string {
  return n === 1 ? `1 ${one}` : `${n} ${many}`;
}

/** The short outcome shown next to the buttons */
export function summaryHeadline(s: Summary): string {
  if (s.cancelled && s.bundled) {
    return 'Cancelled, no bundle written';
  }
  if (s.cancelled) {
    return `Cancelled, ${s.saved} saved`;
  }
  if (s.error !== '') {
    return 'The bundle could not be written';
  }
  if (s.bundled && s.bundle !== '') {
    return `Saved ${s.bundle}`;
  }

  const parts: string[] = [];
  if (s.saved > 0) {
    parts.push(`${s.saved} saved`);
  }
  if (s.failures.length > 0) {
    parts.push(`${s.failures.length} failed`);
  }
  if (s.skipped > 0) {
    parts.push(`${s.skipped} skipped`);
  }

  return parts.length === 0 ? 'Nothing was saved' : parts.join(', ');
}

/** The one sentence that says where a finished run's chapters went */
export function summarySentence(s: Summary): string {
  if (s.cancelled && s.bundled) {
    return 'Cancelled. No bundle was written.';
  }
  if (s.cancelled && s.saved === 0) {
    return 'Cancelled before anything was saved.';
  }
  if (s.cancelled) {
    return `Cancelled. ${plural(s.saved, 'chapter', 'chapters')} saved before it stopped, in ${s.dir}.`;
  }
  if (s.error !== '') {
    return 'The bundle could not be written.';
  }
  if (s.bundled && s.bundle !== '') {
    return `Saved ${s.bundle} to ${s.dir} (${plural(s.saved, 'chapter', 'chapters')}).`;
  }
  if (s.saved > 0) {
    return `Saved ${plural(s.saved, 'chapter', 'chapters')} to ${s.dir}.`;
  }

  return 'Nothing was saved.';
}

/** The status text of a progress row */
export function jobStatus(j: Job): string {
  switch (j.state) {
    case 'downloading':
      return j.status !== '' ? j.status : 'downloading';
    case 'archiving':
      return j.status !== '' ? j.status : 'archiving';
    case 'done':
      return 'done';
    case 'failed':
      return `failed: ${j.reason}`;
    case 'skipped':
      return `skipped: ${j.reason}`;
  }
}

/** The short status at the end of a progress row: phase and percentage while it moves, the outcome after */
export function jobShortStatus(j: Job): string {
  switch (j.state) {
    case 'downloading':
    case 'archiving': {
      const phase =
        j.status !== '' ? j.status.replace(/\.+$/, '') : j.state === 'archiving' ? 'Packing' : 'Downloading';
      return `${phase} · ${Math.round(jobFraction(j) * 100)}%`;
    }
    case 'done':
      return 'Saved';
    case 'failed':
      return 'Failed';
    case 'skipped':
      return 'Skipped';
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
export function errorMessage(e: unknown): string {
  if (typeof e === 'string') {
    return e;
  }
  if (e instanceof Error) {
    return e.message;
  }

  return String(e);
}
