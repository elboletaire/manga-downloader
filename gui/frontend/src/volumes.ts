// The Volumes box: chapter ranges, one group per volume ("1-7; 8-16"), the
// same syntax as --volumes. The backend is the authority (it refuses what
// doesn't parse, and groups the run); this is the same grouping for the
// chapter list to show, before the run.

import { formatNumber } from './run';
import type { Chapter } from './types';

export interface Range {
  begin: number;
  end: number;
}

/** A parsed Volumes box: the site's volumes, or the chapter ranges of each volume (volume N is groups[N-1]) */
export type VolumeSpec = { auto: true } | { auto: false; groups: Range[][] };

const PART = /^(\d+(?:\.\d+)?)(?:-(\d+(?:\.\d+)?))?$/;

/** Parses "1-7,9" as the chapter range box does; a reversed range keeps its first bound */
function parseRanges(text: string): Range[] {
  return text.split(',').map((raw) => {
    const part = raw.trim();
    const m = PART.exec(part);
    if (m === null) {
      throw new Error(`invalid range "${part}"`);
    }
    const begin = Number(m[1]);
    const end = m[2] === undefined ? begin : Math.max(begin, Number(m[2]));
    return { begin, end };
  });
}

function rangeText(r: Range): string {
  return r.begin === r.end ? formatNumber(r.begin) : `${formatNumber(r.begin)}-${formatNumber(r.end)}`;
}

/**
 * Parses the Volumes box. Empty or "auto" is the site's volumes. Otherwise
 * groups are separated by ";", and an empty group keeps the numbering going
 * ("1-7;;20" has no volume 2). Groups can't overlap. Throws with the reason
 * when the text doesn't parse, in the backend's words.
 */
export function parseVolumes(text: string): VolumeSpec {
  const trimmed = text.trim();
  if (trimmed === '' || trimmed.toLowerCase() === 'auto') {
    return { auto: true };
  }
  const segments = trimmed.split(';');
  while (segments.length > 0 && segments[segments.length - 1].trim() === '') {
    segments.pop();
  }
  if (segments.length === 0) {
    throw new Error('no volumes given');
  }
  const groups = segments.map((seg, i) => {
    if (seg.trim() === '') {
      return [];
    }
    try {
      return parseRanges(seg);
    } catch (e) {
      throw new Error(`volume ${i + 1}: ${(e as Error).message}`);
    }
  });
  for (let i = 0; i < groups.length; i++) {
    for (let j = i + 1; j < groups.length; j++) {
      for (const a of groups[i]) {
        for (const b of groups[j]) {
          if (a.begin <= b.end && b.begin <= a.end) {
            throw new Error(`volumes ${i + 1} and ${j + 1} overlap (${rangeText(a)} and ${rangeText(b)})`);
          }
        }
      }
    }
  }

  return { auto: false, groups };
}

export interface VolumeGroup {
  number: number;
  /** In list order, which is by number */
  chapters: Chapter[];
}

export interface Grouping {
  /** The volumes holding at least one chapter, by number */
  volumes: VolumeGroup[];
  /** The chapters no volume holds, packed one file each */
  loose: Chapter[];
}

/** Groups the chapter list into the spec's volumes */
export function groupChapters(chapters: Chapter[], spec: VolumeSpec): Grouping {
  const byNumber = new Map<number, Chapter[]>();
  const loose: Chapter[] = [];
  for (const ch of chapters) {
    const n = spec.auto ? ch.volume : groupOf(ch.number, spec.groups);
    if (n <= 0) {
      loose.push(ch);
      continue;
    }
    const list = byNumber.get(n);
    if (list) {
      list.push(ch);
    } else {
      byNumber.set(n, [ch]);
    }
  }
  const volumes = [...byNumber.entries()]
    .sort(([a], [b]) => a - b)
    .map(([number, list]) => ({ number, chapters: list }));

  return { volumes, loose };
}

function groupOf(number: number, groups: Range[][]): number {
  for (let i = 0; i < groups.length; i++) {
    if (groups[i].some((r) => number >= r.begin && number <= r.end)) {
      return i + 1;
    }
  }
  return 0;
}

/** "chapters 1–7", or "chapter 5" for a volume of one number */
export function chapterSpan(chapters: Chapter[]): string {
  if (chapters.length === 0) {
    return 'no chapters';
  }
  const first = formatNumber(chapters[0].number);
  const last = formatNumber(chapters[chapters.length - 1].number);
  return first === last ? `chapter ${first}` : `chapters ${first}–${last}`;
}

/** A row of the grouped list: a group's header, or one of its chapters */
export type GroupedRow =
  | { kind: 'group'; key: number; label: string; chapters: Chapter[]; collapsed: boolean }
  | { kind: 'chapter'; chapter: Chapter };

/** The key of the "Not in a volume" group; volumes key by their number */
export const LOOSE = 0;

/**
 * Flattens a grouping into list rows: each volume's header followed by its
 * chapters, unless collapsed, and the chapters outside every volume last,
 * under "Not in a volume".
 */
export function groupedRows(grouping: Grouping, collapsed: ReadonlySet<number>): GroupedRow[] {
  const rows: GroupedRow[] = [];
  const add = (key: number, label: string, chapters: Chapter[]) => {
    const isCollapsed = collapsed.has(key);
    rows.push({ kind: 'group', key, label, chapters, collapsed: isCollapsed });
    if (!isCollapsed) {
      for (const chapter of chapters) {
        rows.push({ kind: 'chapter', chapter });
      }
    }
  };
  for (const v of grouping.volumes) {
    add(v.number, `Volume ${v.number}`, v.chapters);
  }
  if (grouping.loose.length > 0) {
    add(LOOSE, 'Not in a volume', grouping.loose);
  }

  return rows;
}

/** How many of the chapters are selected */
export function selectedIn(chapters: Chapter[], chosen: ReadonlySet<number>): number {
  let n = 0;
  for (const ch of chapters) {
    if (chosen.has(ch.index)) {
      n++;
    }
  }
  return n;
}

/**
 * The site's volumes as a Volumes box ("1-7; 8-16"), as the backend writes
 * them (core.SiteVolumes): each group lists runs of consecutive chapters, a
 * missing volume keeps its place as an empty group, and a number listed twice
 * stays in the first volume it shows up in. "" when no chapter has a volume.
 * chapters must be in list order. The mock uses it; the app gets it from Go.
 */
export function siteVolumes(chapters: Chapter[]): string {
  const volumeAt = chapters.map(() => 0);
  const owner = new Map<number, number>();
  let max = 0;
  chapters.forEach((ch, i) => {
    if (ch.volume <= 0) {
      return;
    }
    const o = owner.get(ch.number);
    if (o !== undefined && o !== ch.volume) {
      return;
    }
    owner.set(ch.number, ch.volume);
    volumeAt[i] = ch.volume;
    max = Math.max(max, ch.volume);
  });
  if (max === 0) {
    return '';
  }

  const groups: string[][] = Array.from({ length: max }, () => []);
  for (let i = 0; i < chapters.length;) {
    const v = volumeAt[i];
    if (v === 0) {
      i++;
      continue;
    }
    let j = i;
    while (
      j + 1 < chapters.length &&
      (volumeAt[j + 1] === v || (volumeAt[j + 1] === 0 && owner.get(chapters[j + 1].number) === v))
    ) {
      j++;
    }
    groups[v - 1].push(rangeText({ begin: chapters[i].number, end: chapters[j].number }));
    i = j + 1;
  }

  return groups.map((g) => g.join(',')).join('; ');
}
