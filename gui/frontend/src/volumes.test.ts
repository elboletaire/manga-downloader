import { describe, expect, it } from 'vitest';
import {
  chapterSpan,
  groupChapters,
  groupedRows,
  groupParts,
  isSiteVolumes,
  LOOSE,
  listedVolumes,
  parseVolumes,
  partRows,
  selectedIn,
  siteVolumes,
  volumesSummary,
} from './volumes';
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
    const rows = groupedRows(g, new Set(['volume-1', LOOSE]));
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

describe('groupParts', () => {
  const part = (index: number, number: number, group = ''): Chapter => ({ ...ch(number, 0, index), group });
  // 1 whole, 2 in two parts, 3 by two groups (Alpha in two parts)
  const parts = [part(0, 1), part(1, 2), part(2, 2), part(3, 3, 'Alpha'), part(4, 3, 'Beta'), part(5, 3, 'Alpha')];

  it('merges the chapters sharing a number and a group, in list order', () => {
    expect(groupParts(parts).map((g) => [g.number, g.group, g.chapters.map((c) => c.index)])).toEqual([
      [1, '', [0]],
      [2, '', [1, 2]],
      [3, 'Alpha', [3, 5]],
      [3, 'Beta', [4]],
    ]);
  });

  it('heads a chapter with parts, and leaves one without them a plain row', () => {
    const rows = partRows(groupParts(parts), new Set());
    expect(
      rows.map((r) =>
        r.kind === 'group' ? `${r.label} · ${r.detail}${r.tag ? ` [${r.tag}]` : ''}` : `${r.chapter.index}${r.part ? ` ${r.part.n}/${r.part.of} ${r.part.label}` : ''}${r.inGroup ? '' : ' alone'}`,
      ),
    ).toEqual([
      '0 alone',
      'Chapter 2 · 2 parts',
      '1 1/2 Chapter 2',
      '2 2/2 Chapter 2',
      'Chapter 3 · 2 parts [Alpha]',
      '3 1/2 Chapter 3',
      '5 2/2 Chapter 3',
      '4 alone',
    ]);
  });

  it('folds a chapter to its header', () => {
    const rows = partRows(groupParts(parts), new Set(['part-1']));
    expect(rows.filter((r) => r.kind === 'chapter').map((r) => (r.kind === 'chapter' ? r.chapter.index : -1))).toEqual([
      0, 3, 5, 4,
    ]);
    expect(rows[1]).toMatchObject({ kind: 'group', collapsed: true });
  });

  it("heads a chapter with its title and labels its rows by their part", () => {
    const titled = (index: number, number: number, title: string, part?: string, wholeTitle?: string): Chapter => ({
      ...ch(number, 0, index),
      title,
      ...(part ? { part, wholeTitle } : {}),
    });
    const rows = partRows(
      groupParts([
        titled(0, 4, "Part 1 Un malson que s'esvaeix", 'Part 1', "Un malson que s'esvaeix"),
        titled(1, 4, 'Part 2 Un altre títol', 'Part 2', 'Un altre títol'),
        titled(2, 5, 'Chapter 5 前編', '前編', ''),
        titled(3, 5, 'Chapter 5 後編 Part 1', '後編 Part 1', ''),
        titled(4, 6, 'Foo, first half'),
        titled(5, 6, 'Foo, second half'),
      ]),
      new Set(),
    );
    expect(rows.map((r) => (r.kind === 'group' ? `${r.label} · ${r.detail}` : r.part?.label))).toEqual([
      // the first part's title, as core names the merged file
      "Chapter 4 · Un malson que s'esvaeix · 2 parts",
      'Part 1',
      'Part 2',
      'Chapter 5 · 2 parts',
      '前編',
      '後編 Part 1',
      // without a part from the grabber, the titles as they are
      'Chapter 6 · Foo, first half · 2 parts',
      'Foo, first half',
      'Foo, second half',
    ]);
  });
});

describe('shared parts with Go', () => {
  // the cases core.GroupVolumes runs too, so the list and the run group the same way
  it.each(shared.parts.map((c) => [c.name, c] as const))('merges the parts: %s', (_, c) => {
    const chapters = c.chapters.map((p, i) => ({ ...ch(p.number, 0, i), group: p.group, language: p.language }));
    const files = groupParts(chapters).map((g) => g.chapters.map((x) => x.index));
    expect(files.sort((a, b) => a[0] - b[0])).toEqual(c.want);
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
    ['repeated number across volumes gives no ranges', [ch(1, 1), ch(1, 2), ch(2, 1), ch(2, 2)], ''],
    ['mixed volume tags on one number give no ranges', [ch(1, 1), ch(2, 1), ch(2, 0), ch(3, 1)], ''],
  ])('%s', (_, chapters, want) => {
    expect(siteVolumes(chapters)).toBe(want);
  });

  it.each([
    ['none', [ch(1), ch(2)], 0],
    ['repeated number across volumes', [ch(1, 1), ch(1, 2), ch(2, 1), ch(2, 2)], 2],
    ['mixed volume tags on one number', [ch(1, 1), ch(2, 1), ch(2, 0), ch(3, 1)], 1],
  ])('lists the volumes: %s', (_, chapters, want) => {
    expect(listedVolumes(chapters)).toBe(want);
  });

  it('recognises the site volumes box', () => {
    expect(['', '  ', 'auto', 'AUTO'].map(isSiteVolumes)).toEqual([true, true, true, true]);
    expect(isSiteVolumes('1-7/8-16')).toBe(false);
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
    expect(listedVolumes(chapters)).toBe(c.listed);
    if (c.want === '') {
      return;
    }
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

describe('volumesSummary', () => {
  // the four branches: own ranges, the site's ranges, listed but not writable, none listed
  it.each([
    { name: 'own ranges', text: '1-7/8-16', site: '', listed: 2, volumes: 2, loose: 0, want: { text: '2 volumes' } },
    {
      name: 'own ranges, outside chapters',
      text: '1-7',
      site: '',
      listed: 1,
      volumes: 1,
      loose: 3,
      want: { text: '1 volume, 3 chapters outside them' },
    },
    {
      name: 'own ranges, none falls in',
      text: '1-7',
      site: '',
      listed: 1,
      volumes: 0,
      loose: 0,
      want: { text: 'No chapter falls in these volumes.' },
    },
    {
      name: 'site ranges',
      text: '',
      site: '61:598-603/604-614',
      listed: 55,
      volumes: 55,
      loose: 15,
      want: { text: "Site's volumes: 55, 15 chapters outside them", ranges: '61:598-603/604-614' },
    },
    {
      name: 'auto is the site',
      text: 'auto',
      site: '1-2/3-4',
      listed: 2,
      volumes: 2,
      loose: 0,
      want: { text: "Site's volumes: 2", ranges: '1-2/3-4' },
    },
    {
      name: 'listed, not writable',
      text: '',
      site: '',
      listed: 9,
      volumes: 7,
      loose: 1,
      want: {
        text: "9 volumes, as the site files them (they can't be written as chapter ranges), 1 chapter outside them",
      },
    },
    {
      name: 'none listed',
      text: '',
      site: '',
      listed: 0,
      volumes: 0,
      loose: 0,
      want: { text: 'The site lists no volumes: enter them as chapter ranges.' },
    },
  ])('$name', ({ text, site, listed, volumes, loose, want }) => {
    expect(volumesSummary(text, site, listed, volumes, loose)).toEqual(want);
  });
});
