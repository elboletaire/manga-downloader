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

/** The full story of a finished run, one entry per line */
export function summaryLines(s: Summary): string[] {
  const lines: string[] = [];
  if (s.cancelled && s.bundled) {
    lines.push('Cancelled. No bundle was written.');
  } else if (s.cancelled && s.saved === 0) {
    lines.push('Cancelled before anything was saved.');
  } else if (s.cancelled) {
    lines.push(`Cancelled. ${plural(s.saved, 'chapter', 'chapters')} saved before it stopped, in ${s.dir}.`);
  } else if (s.error !== '') {
    lines.push('The bundle could not be written.');
  } else if (s.bundled && s.bundle !== '') {
    lines.push(`Saved ${s.bundle} to ${s.dir} (${plural(s.saved, 'chapter', 'chapters')}).`);
  } else if (s.saved > 0) {
    lines.push(`Saved ${plural(s.saved, 'chapter', 'chapters')} to ${s.dir}.`);
  } else {
    lines.push('Nothing was saved.');
  }

  if (s.skipped > 0) {
    lines.push(`${plural(s.skipped, 'chapter', 'chapters')} skipped: --skip-pages excluded every page.`);
  }
  if (s.failures.length > 0) {
    lines.push(`${s.failures.length} failed:`);
    for (const f of s.failures) {
      lines.push(`  - ${f}`);
    }
  }

  return lines;
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
