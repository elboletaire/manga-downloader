import { describe, expect, it } from 'vitest';
import { chapterSpan, groupChapters, groupedRows, LOOSE, parseVolumes, selectedIn, siteVolumes } from './volumes';
import type { Chapter } from './types';
import shared from '../../../ranges/testdata/volumes.json';

const ch = (number: number, volume = 0, index = number): Chapter => ({
  index,
  number,
  title: `Chapter ${number}`,
  language: '',
  group: '',
  volume,
  locked: false,
});

// chapters 1, 2 (vol 1), 2.5 (none), 3, 4 (vol 2), 5 (none), 6 (vol 4)
const list = [ch(1, 1, 0), ch(2, 1, 1), ch(2.5, 0, 2), ch(3, 2, 3), ch(4, 2, 4), ch(5, 0, 5), ch(6, 4, 6)];

const numbers = (chapters: Chapter[]) => chapters.map((c) => c.number);

describe('parseVolumes', () => {
  it.each([
    ['', true],
    ['  ', true],
    ['auto', true],
    ['AUTO', true],
  ])('reads %j as the site volumes', (text, auto) => {
    expect(parseVolumes(text).auto).toBe(auto);
  });

  it.each([
    ['1-7/8-16', [[{ begin: 1, end: 7 }], [{ begin: 8, end: 16 }]]],
    [' 1-7 / 8-16 /', [[{ begin: 1, end: 7 }], [{ begin: 8, end: 16 }]]],
    [
      '1-8,10/9',
      [
        [
          { begin: 1, end: 8 },
          { begin: 10, end: 10 },
        ],
        [{ begin: 9, end: 9 }],
      ],
    ],
    ['168.1-170/262.5', [[{ begin: 168.1, end: 170 }], [{ begin: 262.5, end: 262.5 }]]],
    ['1-7//20', [[{ begin: 1, end: 7 }], [], [{ begin: 20, end: 20 }]]],
    ['10-5', [[{ begin: 10, end: 10 }]]],
  ])('parses %j', (text, groups) => {
    expect(parseVolumes(text)).toEqual({ auto: false, groups });
  });

  it.each([
    ['1-7/7-16', 'volumes 1 and 2 overlap (1-7 and 7-16)'],
    ['1-3/5/2.5', 'volumes 1 and 3 overlap (1-3 and 2.5)'],
    ['1-7/abc', 'volume 2: invalid range "abc"'],
    ['1-7/-3', 'volume 2: invalid range "-3"'],
    [' / /', 'no volumes given'],
  ])('refuses %j', (text, message) => {
    expect(() => parseVolumes(text)).toThrow(message);
  });
});

describe('groupChapters', () => {
  it('groups by the site volumes', () => {
    const g = groupChapters(list, parseVolumes(''));
    expect(g.volumes.map((v) => [v.number, numbers(v.chapters)])).toEqual([
      [1, [1, 2]],
      [2, [3, 4]],
      [4, [6]],
    ]);
    expect(numbers(g.loose)).toEqual([2.5, 5]);
  });

  it('groups by chapter ranges, numbering by position', () => {
    const g = groupChapters(list, parseVolumes('1-2//3-6'));
    expect(g.volumes.map((v) => [v.number, numbers(v.chapters)])).toEqual([
      [1, [1, 2]],
      [3, [3, 4, 5, 6]],
    ]);
    expect(numbers(g.loose)).toEqual([2.5]);
  });

  it('leaves out a volume that holds nothing', () => {
    const g = groupChapters(list, parseVolumes('1-2/100'));
    expect(g.volumes.map((v) => v.number)).toEqual([1]);
    expect(g.loose).toHaveLength(5);
  });
});

describe('groupedRows', () => {
  const g = groupChapters(list, parseVolumes('1-2/3-4'));

  it('puts each volume header before its chapters, and the rest last', () => {
    const rows = groupedRows(g, new Set());
    expect(rows.map((r) => (r.kind === 'group' ? r.label : r.chapter.number))).toEqual([
      'Volume 1',
      1,
      2,
      'Volume 2',
      3,
      4,
      'Not in a volume',
      2.5,
      5,
      6,
    ]);
  });

  it('hides the chapters of a collapsed group, keeping its header', () => {
    const rows = groupedRows(g, new Set([1, LOOSE]));
    expect(rows.map((r) => (r.kind === 'group' ? `${r.label}${r.collapsed ? ' +' : ''}` : r.chapter.number))).toEqual([
      'Volume 1 +',
      'Volume 2',
      3,
      4,
      'Not in a volume +',
    ]);
  });

  it('has no "Not in a volume" group when every chapter has a volume', () => {
    const rows = groupedRows(groupChapters(list, parseVolumes('1-10')), new Set());
    expect(rows.filter((r) => r.kind === 'group')).toHaveLength(1);
  });
});

describe('helpers', () => {
  it('names the span of a volume', () => {
    expect(chapterSpan([ch(1), ch(7.5)])).toBe('chapters 1–7.5');
    expect(chapterSpan([ch(5)])).toBe('chapter 5');
  });

  it('counts the selected chapters of a group', () => {
    expect(selectedIn(list, new Set([0, 3, 99]))).toBe(2);
  });
});

describe('siteVolumes', () => {
  // the same cases as core's TestSiteVolumes, so the mock and the app agree
  it.each([
    ['none', [ch(1), ch(2)], ''],
    ['contiguous', [ch(1, 1), ch(2, 1), ch(3, 2), ch(4, 2)], '1-2/3-4'],
    ['single chapter volume', [ch(1, 1), ch(2, 2)], '1/2'],
    ['loose chapter breaks the run', [ch(1, 1), ch(1.5), ch(2, 1), ch(3, 2)], '1,2/3'],
    ['missing volume is named', [ch(1, 1), ch(2, 3)], '1/3:2'],
    ['loose tail', [ch(1, 1), ch(2, 1), ch(3), ch(4)], '1-2'],
    ['repeated number stays in its first volume', [ch(1, 1), ch(2, 1), ch(2, 2), ch(3, 2)], '1-2/3'],
  ])('%s', (_, chapters, want) => {
    expect(siteVolumes(chapters)).toBe(want);
  });
});

// the cases ranges.ParseVolumes and core.SiteVolumes run too, so Go and the app agree
describe('shared with Go', () => {
  it.each(shared.parse.map((c) => [c.in, c] as const))('parses %j', (text, c) => {
    if ('error' in c && c.error !== undefined) {
      expect(() => parseVolumes(text)).toThrow(c.error);
      return;
    }
    const spec = parseVolumes(text);
    if (spec.auto) {
      throw new Error('parsed as the site volumes');
    }
    const got: Record<string, number[][]> = {};
    spec.groups.forEach((g, i) => {
      if (g.length > 0) {
        got[String(i + 1)] = g.map((r) => [r.begin, r.end]);
      }
    });
    expect(got).toEqual('volumes' in c ? c.volumes : undefined);
  });

  it.each(shared.site.map((c) => [c.name, c] as const))('writes the site volumes: %s', (_, c) => {
    const chapters = c.chapters.map(([number, volume], i) => ch(number, volume, i));
    expect(siteVolumes(chapters)).toBe(c.want);
    // and what it writes groups each chapter where the site does
    const g = groupChapters(chapters, parseVolumes(c.want));
    expect(g.volumes.flatMap((v) => v.chapters.map((x) => [x.number, v.number]))).toEqual(
      chapters.filter((x) => x.volume > 0).map((x) => [x.number, x.volume]),
    );
  });
});

describe('named volumes', () => {
  it('groups by named volumes', () => {
    const g = groupChapters(list, parseVolumes('4: 1-2/3-6'));
    expect(g.volumes.map((v) => [v.number, numbers(v.chapters)])).toEqual([
      [4, [1, 2]],
      [5, [3, 4, 5, 6]],
    ]);
  });
});
