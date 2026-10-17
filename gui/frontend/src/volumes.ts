// The Volumes box: chapter ranges, one group per volume ("1-7/8-16"), the
// same syntax as --volumes. The backend is the authority (it refuses what
// doesn't parse, and groups the run); this is the same grouping for the
// chapter list to show, before the run. A parts run (--volumes parts) has
// no box: groupParts is its grouping.

import { formatNumber, plural } from './format';
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

/** The highest volume number, as volumes are indexed by it */
const MAX_VOLUME = 9999;

/** A group naming its volume: "16:112-114" */
const VOLUME_PREFIX = /^([^:]*):(.*)$/s;

/** The N of an "N:" prefix, a whole number from 1 (grabbers' volumes are integers) */
function volumeNumber(text: string): number {
  if (!/^\d+$/.test(text)) {
    throw new Error(`invalid volume number "${text}", it must be a whole number like 4`);
  }
  const n = Number(text);
  if (n > MAX_VOLUME) {
    throw new Error(`invalid volume number "${text}", volume numbers go up to ${MAX_VOLUME}`);
  }
  if (n === 0) {
    throw new Error('invalid volume number "0", volumes start at 1');
  }
  return n;
}

/** Whether the Volumes box stands for the site's own volumes: empty or "auto" */
export function isSiteVolumes(text: string): boolean {
  const t = text.trim().toLowerCase();
  return t === '' || t === 'auto';
}

/**
 * Parses the Volumes box. Empty or "auto" is the site's volumes. Otherwise
 * groups are separated by "/". A group can name its volume ("16:112-114");
 * one without continues from the previous group's number, and an empty group
 * keeps the numbering going ("1-7//20" has no volume 2). A volume can't be
 * given twice and groups can't overlap. Throws with the reason when the text
 * doesn't parse, in the backend's words (ranges.ParseVolumes).
 */
export function parseVolumes(text: string): VolumeSpec {
  if (isSiteVolumes(text)) {
    return { auto: true };
  }
  const trimmed = text.trim();
  const segments = trimmed.split('/');
  while (segments.length > 0 && segments[segments.length - 1].trim() === '') {
    segments.pop();
  }
  if (segments.length === 0) {
    throw new Error('no volumes given');
  }
  const groups: Range[][] = [];
  const given = new Set<number>();
  let number = 0;
  for (const raw of segments) {
    number++;
    let seg = raw.trim();
    if (seg === '') {
      continue;
    }
    const m = VOLUME_PREFIX.exec(seg);
    if (m !== null) {
      try {
        number = volumeNumber(m[1].trim());
      } catch (e) {
        throw new Error(`volume ${number}: ${(e as Error).message}`);
      }
      seg = m[2].trim();
      if (seg === '') {
        throw new Error(`volume ${number}: no chapters given`);
      }
    }
    if (number > MAX_VOLUME) {
      throw new Error(`volume ${number}: volume numbers go up to ${MAX_VOLUME}`);
    }
    let rngs: Range[];
    try {
      rngs = parseRanges(seg);
    } catch (e) {
      throw new Error(`volume ${number}: ${(e as Error).message}`);
    }
    while (groups.length < number) {
      groups.push([]);
    }
    if (given.has(number)) {
      throw new Error(`volume ${number} is given twice`);
    }
    given.add(number);
    groups[number - 1] = rngs;
  }
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

/** The Volumes box's note: a line, and the site's ranges to set at its end (cut with an ellipsis) */
export interface VolumesSummary {
  text: string;
  ranges?: string;
}

/**
 * The note under the Volumes box. With the box on the site's volumes, the
 * ranges the site's volumes write as go at the end when they can, else the
 * count of volumes the site lists; a box of the user's own gets the count.
 */
export function volumesSummary(
  text: string,
  site: string,
  listed: number,
  volumes: number,
  loose: number,
): VolumesSummary {
  const outside = loose > 0 ? `, ${plural(loose, 'chapter', 'chapters')} outside them` : '';
  if (!isSiteVolumes(text)) {
    return {
      text: volumes === 0 ? 'No chapter falls in these volumes.' : `${plural(volumes, 'volume', 'volumes')}${outside}`,
    };
  }
  if (site !== '') {
    return { text: `Site's volumes: ${volumes}${outside}`, ranges: site };
  }
  if (listed > 0) {
    return {
      text: `${plural(listed, 'volume', 'volumes')}, as the site files them (they can't be written as chapter ranges)${outside}`,
    };
  }
  return { text: 'The site lists no volumes: enter them as chapter ranges.' };
}

/** A row of the grouped list: a group's header, or a chapter (in a group or on its own) */
export type GroupedRow =
  | { kind: 'group'; key: string; label: string; detail: string; tag: string; chapters: Chapter[]; collapsed: boolean }
  | { kind: 'chapter'; chapter: Chapter; inGroup: boolean; part?: { n: number; of: number } };

/** The key of the "Not in a volume" group; volumes key by "volume-N" */
export const LOOSE = 'loose';

/** The rows of a list shown ungrouped */
export function plainRows(chapters: Chapter[]): GroupedRow[] {
  return chapters.map((chapter) => ({ kind: 'chapter', chapter, inGroup: false }));
}

/**
 * Flattens a grouping into list rows: each volume's header followed by its
 * chapters, unless collapsed, and the chapters outside every volume last,
 * under "Not in a volume".
 */
export function groupedRows(grouping: Grouping, collapsed: ReadonlySet<string>): GroupedRow[] {
  const rows: GroupedRow[] = [];
  const add = (key: string, label: string, detail: string, chapters: Chapter[]) => {
    const isCollapsed = collapsed.has(key);
    rows.push({ kind: 'group', key, label, detail, tag: '', chapters, collapsed: isCollapsed });
    if (!isCollapsed) {
      for (const chapter of chapters) {
        rows.push({ kind: 'chapter', chapter, inGroup: true });
      }
    }
  };
  for (const v of grouping.volumes) {
    add(`volume-${v.number}`, `Volume ${v.number}`, chapterSpan(v.chapters), v.chapters);
  }
  if (grouping.loose.length > 0) {
    add(LOOSE, 'Not in a volume', plural(grouping.loose.length, 'chapter', 'chapters'), grouping.loose);
  }

  return rows;
}

/** A chapter's file in a parts run: its parts, or the chapter alone when it has none */
export interface PartGroup {
  number: number;
  group: string;
  /** In list order */
  chapters: Chapter[];
}

/**
 * Groups the chapters a parts run merges (--volumes parts): those sharing a
 * number and a group, so two groups' releases never merge. Files keep the
 * list order, at their first part.
 */
export function groupParts(chapters: Chapter[]): PartGroup[] {
  const out: PartGroup[] = [];
  const byKey = new Map<string, PartGroup>();
  for (const ch of chapters) {
    const key = `${ch.number}\u0000${ch.group}`;
    const g = byKey.get(key);
    if (g) {
      g.chapters.push(ch);
    } else {
      const next = { number: ch.number, group: ch.group, chapters: [ch] };
      byKey.set(key, next);
      out.push(next);
    }
  }

  return out;
}

/**
 * The list rows of a parts run: a chapter with parts is a header followed by
 * its parts, unless collapsed; a chapter without parts stays a plain row.
 */
export function partRows(groups: PartGroup[], collapsed: ReadonlySet<string>): GroupedRow[] {
  const rows: GroupedRow[] = [];
  for (const g of groups) {
    if (g.chapters.length === 1) {
      rows.push({ kind: 'chapter', chapter: g.chapters[0], inGroup: false });
      continue;
    }
    const key = `part-${g.chapters[0].index}`;
    const isCollapsed = collapsed.has(key);
    rows.push({
      kind: 'group',
      key,
      label: `Chapter ${formatNumber(g.number)}`,
      detail: `${g.chapters.length} parts`,
      tag: g.group,
      chapters: g.chapters,
      collapsed: isCollapsed,
    });
    if (!isCollapsed) {
      g.chapters.forEach((chapter, i) => {
        rows.push({ kind: 'chapter', chapter, inGroup: true, part: { n: i + 1, of: g.chapters.length } });
      });
    }
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
 * The site's volumes as core.SiteVolumes writes them, "" when they can't be
 * written so. chapters must be in list order.
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

  // name a volume only where counting on from the previous one wouldn't land on it
  const parts: string[] = [];
  let prev = 0;
  groups.forEach((g, i) => {
    if (g.length === 0) {
      return;
    }
    const n = i + 1;
    parts.push(n === prev + 1 ? g.join(',') : `${n}:${g.join(',')}`);
    prev = n;
  });

  const text = parts.join('/');
  return groupsAsSite(text, chapters) ? text : '';
}

/** Whether the Volumes box text groups each chapter into the volume the site files it under, as auto does */
function groupsAsSite(text: string, chapters: Chapter[]): boolean {
  let spec: VolumeSpec;
  try {
    spec = parseVolumes(text);
  } catch {
    return false;
  }
  if (spec.auto) {
    return false;
  }
  return chapters.every((ch) => groupOf(ch.number, spec.groups) === Math.max(ch.volume, 0));
}

/** How many volumes the site files the chapters under, 0 when none does (core.ListedVolumes) */
export function listedVolumes(chapters: Chapter[]): number {
  return new Set(chapters.filter((ch) => ch.volume > 0).map((ch) => ch.volume)).size;
}
