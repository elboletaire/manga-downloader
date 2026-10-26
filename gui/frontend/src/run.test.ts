import { describe, expect, it } from 'vitest';
import { createT } from './i18n';
import { createMockApi } from './mock';
import {
  downloadBlocker,
  downloadLabel,
  hostOf,
  overallProgress,
  summaryTone,
  tally,
} from './run';
import type { Job, State, Summary } from './types';

const t = createT('en');

const job = (over: Partial<Job>): Job => ({
  title: 'Ch',
  tags: '',
  number: 1,
  total: 20,
  done: 0,
  state: 'downloading',
  reason: '',
  status: '',
  volume: false,
  ...over,
});

const summary: Summary = {
  dir: '/out',
  cancelled: false,
  saved: 0,
  volumes: 0,
  parts: false,
  bundled: false,
  bundle: '',
  skipped: 0,
  existing: 0,
  failures: [],
  error: '',
};

async function loadedState(over: Partial<State> = {}): Promise<State> {
  return { ...(await createMockApi('loaded').state()), ...over };
}

describe('overallProgress', () => {
  it('counts chapters that have no job yet as queued', () => {
    const o = overallProgress({
      run: { chapters: 4, bundle: false, volumes: false, rows: 0 },
      jobs: [job({ state: 'done', done: 20 }), job({ done: 10 })],
    });
    expect(o).toEqual({ finished: 1, total: 4, queued: 2, fraction: 1.5 / 4 });
  });

  it('never moves back through a run by volumes', () => {
    // the rows the backend reports along a run of two volumes and one loose
    // chapter: a volume's row counts out of a fixed total, the loose
    // chapter's row only appears once it starts
    const run = { chapters: 5, bundle: false, volumes: true, rows: 3 };
    const vol = (n: number, over: Partial<Job>) =>
      job({ title: `Vol. 0${n}`, number: n, total: 10000, volume: true, status: 'Waiting...', ...over });
    const loose = (over: Partial<Job>) => job({ title: 'Chapter 5', number: 5, total: 6, ...over });
    const steps: Job[][] = [
      [vol(1, {}), vol(2, {})],
      [vol(1, { status: '', done: 1250 }), vol(2, {})],
      [vol(1, { status: '', done: 2500 }), vol(2, { status: '', done: 500 })],
      [vol(1, { status: '', done: 2500 }), vol(2, { status: '', done: 500 }), loose({ done: 1 })],
      [vol(1, { state: 'archiving', status: 'Packing...', done: 5000 }), vol(2, { done: 2500 }), loose({ done: 3 })],
      [vol(1, { state: 'archiving', status: 'Packing...', done: 8333 }), vol(2, { done: 5000 }), loose({ done: 5 })],
      [vol(1, { state: 'done', done: 10000 }), vol(2, { state: 'failed', done: 5000 }), loose({ done: 5 })],
      [vol(1, { state: 'done', done: 10000 }), vol(2, { state: 'failed', done: 5000 }), loose({ state: 'done', done: 6 })],
    ];
    const fractions = steps.map((jobs) => overallProgress({ run, jobs }).fraction);
    for (let i = 1; i < fractions.length; i++) {
      expect(fractions[i]).toBeGreaterThanOrEqual(fractions[i - 1]);
    }
    expect(fractions.at(-1)).toBe(1);
  });

  it('counts a failed or skipped chapter as finished', () => {
    const o = overallProgress({
      run: { chapters: 2, bundle: false, volumes: false, rows: 0 },
      jobs: [job({ state: 'failed', done: 3 }), job({ state: 'skipped', total: 0 })],
    });
    expect(o.finished).toBe(2);
    expect(o.fraction).toBe(1);
  });

  it("follows the bundle's single job", () => {
    const o = overallProgress({
      run: { chapters: 9, bundle: true, volumes: false, rows: 0 },
      jobs: [job({ done: 5 })],
    });
    expect(o).toEqual({ finished: 0, total: 1, queued: 0, fraction: 0.25 });
  });

  it('counts the rows of a volume run, not its chapters', () => {
    const o = overallProgress({
      run: { chapters: 12, bundle: false, volumes: true, rows: 3 },
      jobs: [job({ state: 'done', volume: true }), job({ volume: true, done: 5 })],
    });
    expect(o).toEqual({ finished: 1, total: 3, queued: 1, fraction: 1.25 / 3 });
  });

  it('is empty before any run', () => {
    expect(overallProgress({ run: { chapters: 0, bundle: false, volumes: false, rows: 0 }, jobs: [] }).fraction).toBe(
      0,
    );
  });
});

describe('tally', () => {
  it('counts jobs per state', () => {
    const t = tally([job({}), job({ state: 'done' }), job({ state: 'done' })]);
    expect(t.done).toBe(2);
    expect(t.downloading).toBe(1);
    expect(t.failed).toBe(0);
  });
});

describe('downloadBlocker', () => {
  it('lets a loaded series with a selection download', async () => {
    expect(downloadBlocker(await loadedState(), t)).toBe('');
  });

  it('explains each refusal before the click', async () => {
    expect(downloadBlocker(await createMockApi('empty').state(), t)).toMatch(/Load a series/);
    expect(downloadBlocker(await loadedState({ loading: true }), t)).toMatch(/loading/);
    expect(downloadBlocker(await loadedState({ stale: true }), t)).toMatch(/reload/);
    expect(downloadBlocker(await loadedState({ selectedCount: 0 }), t)).toMatch(/Select at least one/);
  });

  it('asks for volumes when the site lists none', async () => {
    const s = await loadedState();
    const volumes = { ...s.options, packAs: 'volumes' as const };
    expect(downloadBlocker({ ...s, options: volumes, volumes: { text: '', site: '', listed: 0 } }, t)).toMatch(
      /enter them as chapter ranges/,
    );
    expect(downloadBlocker({ ...s, options: volumes, volumes: { text: '1-7', site: '', listed: 0 } }, t)).toBe('');
    expect(downloadBlocker({ ...s, options: volumes, volumes: { text: '', site: '1-7', listed: 1 } }, t)).toBe('');
    // a site whose volumes don't write as ranges still packs them, as auto
    expect(downloadBlocker({ ...s, options: volumes, volumes: { text: '', site: '', listed: 2 } }, t)).toBe('');
    expect(downloadBlocker({ ...s, volumes: { text: '', site: '', listed: 0 } }, t)).toBe('');
  });
});

describe('downloadLabel', () => {
  it('translates the label and the refusals', async () => {
    const s = await loadedState({ selectedCount: 5 });
    expect(downloadLabel(s, [], [], createT('es'))).toBe('Descargar 5 capítulos');
    expect(downloadLabel(s, [], [], createT('ca'))).toBe('Descarrega 5 capítols');
    expect(downloadBlocker({ ...s, selectedCount: 0 }, createT('ca'))).toBe('Selecciona almenys un capítol.');
  });

  it('names the count, and the bundle when there is one', async () => {
    const s = await loadedState();
    expect(downloadLabel({ ...s, selectedCount: 0 }, [], [], t)).toBe('Download');
    expect(downloadLabel({ ...s, selectedCount: 1 }, [], [], t)).toBe('Download 1 chapter');
    expect(downloadLabel({ ...s, selectedCount: 5 }, [], [], t)).toBe('Download 5 chapters');
    expect(downloadLabel({ ...s, selectedCount: 5, options: { ...s.options, packAs: 'bundle' } }, [], [], t)).toBe(
      'Download 5 chapters as one file',
    );
  });

  describe('packing as volumes', () => {
    // chapters 1 to 20 at indices 0 to 19; the Volumes box puts 1-7 in volume 1 and 8-16 in volume 2
    const list = Array.from({ length: 20 }, (_, i) => ({
      index: i,
      number: i + 1,
      title: '',
      language: '',
      group: '',
      volume: 0,
      locked: false,
    }));
    const from = (first: number, last: number) => Array.from({ length: last - first + 1 }, (_, i) => first - 1 + i);

    it('names the volumes and their chapters', async () => {
      const s = await loadedState();
      const volumes = { ...s, options: { ...s.options, packAs: 'volumes' as const }, volumes: { text: '1-7/8-16', site: '', listed: 0 } };
      expect(downloadLabel({ ...volumes, selectedCount: 16 }, list, from(1, 16), t)).toBe(
        'Download 2 volumes (16 chapters)',
      );
      expect(downloadLabel({ ...volumes, selectedCount: 7 }, list, from(1, 7), t)).toBe('Download 1 volume (7 chapters)');
    });

    it('names the loose chapters beside the volumes', async () => {
      const s = await loadedState();
      const volumes = { ...s, options: { ...s.options, packAs: 'volumes' as const }, volumes: { text: '1-7/8-16', site: '', listed: 0 } };
      expect(downloadLabel({ ...volumes, selectedCount: 19 }, list, [...from(1, 16), ...from(17, 19)], t)).toBe(
        'Download 2 volumes and 3 chapters',
      );
      expect(downloadLabel({ ...volumes, selectedCount: 8 }, list, [...from(1, 7), 16], t)).toBe(
        'Download 1 volume and 1 chapter',
      );
    });

    it('takes the site volumes when the Volumes box is empty', async () => {
      const s = await loadedState();
      const sited = list.map((ch, i) => ({ ...ch, volume: i < 7 ? 1 : i < 16 ? 2 : 0 }));
      const site = { ...s, options: { ...s.options, packAs: 'volumes' as const }, volumes: { text: '', site: '', listed: 0 } };
      expect(downloadLabel({ ...site, selectedCount: 16 }, sited, from(1, 16), t)).toBe('Download 2 volumes (16 chapters)');
      expect(downloadLabel({ ...site, selectedCount: 17 }, sited, [...from(1, 16), 17], t)).toBe(
        'Download 2 volumes and 1 chapter',
      );
    });

    it('falls back to the chapter count when no selected chapter is in a volume', async () => {
      const s = await loadedState();
      const volumes = { ...s, options: { ...s.options, packAs: 'volumes' as const }, volumes: { text: '1-7/8-16', site: '', listed: 0 } };
      expect(downloadLabel({ ...volumes, selectedCount: 3 }, list, from(17, 19), t)).toBe('Download 3 chapters');
    });

    it('falls back to the chapter count when the Volumes box does not parse', async () => {
      const s = await loadedState();
      const volumes = { ...s, options: { ...s.options, packAs: 'volumes' as const }, volumes: { text: 'nope', site: '', listed: 0 } };
      expect(downloadLabel({ ...volumes, selectedCount: 16 }, list, from(1, 16), t)).toBe('Download 16 chapters');
    });
  });
});

describe('downloadLabel, merging parts', () => {
  // 1 whole, then 2 and 3 in two parts each (indices 1-2 and 3-4)
  const list = [1, 2, 2, 3, 3].map((number, index) => ({
    index,
    number,
    title: '',
    language: '',
    group: '',
    volume: 0,
    locked: false,
  }));

  it('counts the chapters the parts make, and says when some merge', async () => {
    const s = await loadedState();
    const parts = { ...s, options: { ...s.options, packAs: 'parts' as const } };
    expect(downloadLabel({ ...parts, selectedCount: 5 }, list, [0, 1, 2, 3, 4], t)).toBe(
      'Download 3 chapters, parts merged',
    );
    expect(downloadLabel({ ...parts, selectedCount: 3 }, list, [0, 1, 3], t)).toBe('Download 3 chapters');
    expect(downloadLabel({ ...parts, selectedCount: 2 }, list, [1, 2], t)).toBe('Download 1 chapter, parts merged');
  });
});

describe('summaryTone', () => {
  it('maps outcomes to colours', () => {
    expect(summaryTone({ ...summary, saved: 3 })).toBe('success');
    expect(summaryTone({ ...summary, saved: 3, failures: [{ label: '', reason: 'a' }] })).toBe('warning');
    expect(summaryTone({ ...summary, failures: [{ label: '', reason: 'a' }] })).toBe('danger');
    expect(summaryTone({ ...summary, bundled: true, error: 'disk full' })).toBe('danger');
    expect(summaryTone({ ...summary, cancelled: true, saved: 2 })).toBe('neutral');
    expect(summaryTone(summary)).toBe('neutral');
    expect(summaryTone({ ...summary, bundled: true, bundle: 'a.cbz', saved: 3 })).toBe('success');
    expect(summaryTone({ ...summary, volumes: 1, failures: [{ label: '', reason: 'a' }] })).toBe('warning');
  });
});

describe('hostOf', () => {
  it('drops www and survives junk', () => {
    expect(hostOf('https://www.example.com/series/a')).toBe('example.com');
    expect(hostOf(' https://sub.example.org/x ')).toBe('sub.example.org');
    expect(hostOf('not a url')).toBe('');
  });
});
