import { describe, expect, it, vi } from 'vitest';
import { createMockApi, mockSetupFrom, scenarioFrom } from './mock';
import type { State } from './types';

describe('scenarioFrom', () => {
  it('reads the scenario from the query, defaulting to empty', () => {
    expect(scenarioFrom('?mock&scenario=running')).toBe('running');
    expect(scenarioFrom('?scenario=bogus')).toBe('empty');
    expect(scenarioFrom('')).toBe('empty');
  });
});

describe('createMockApi', () => {
  it('starts empty and refuses a download without a series', async () => {
    const api = createMockApi('empty');
    expect((await api.state()).loaded).toBe(false);
    await expect(api.start()).rejects.toBe('load a series first');
  });

  it('loads a series, selects a range and reports the match count', async () => {
    // chapters: 30 loads the newest 30 chapters (1165-1194)
    const api = createMockApi({ chapters: 30 });
    await api.load('https://example.com/series/a');
    const state = await api.state();
    expect(state.loaded).toBe(true);
    expect(state.chapterCount).toBe(30);

    expect(await api.selectRange('1165-1167,1169')).toBe(4);
    expect(await api.selection()).toEqual([0, 1, 2, 4]);
    expect(await api.selectRange('1164')).toBe(0); // ch 1164 is not in the newest 30
  });

  it('rejects an empty selection and an empty URL', async () => {
    const api = createMockApi('loaded');
    await api.setAll(false);
    await expect(api.start()).rejects.toBe('select at least one chapter');
    await expect(api.load('  ')).rejects.toBe('enter the URL of a series first');
  });

  it('marks the list stale when the language changes until it is reloaded', async () => {
    const api = createMockApi('loaded');
    await api.setOptions({ ...(await api.state()).options, language: 'es' });
    expect((await api.state()).stale).toBe(true);
    await api.load((await api.state()).url);
    expect((await api.state()).stale).toBe(false);
  });

  it('selects a set of indices in one call, or none when one is out of range', async () => {
    const api = createMockApi({ chapters: 10 });
    await api.load('https://example.com/series/a');
    await api.setSelectedMany([2, 3, 4], true);
    expect(await api.selection()).toEqual([2, 3, 4]);

    await expect(api.setSelectedMany([0, 99], true)).rejects.toBe('chapter 99 is not in the list');
    expect(await api.selection()).toEqual([2, 3, 4]);

    await api.setSelectedMany([3], false);
    expect(await api.selection()).toEqual([2, 4]);
  });

  it('refuses to change the selection, pick a folder or start while a download runs', async () => {
    const api = createMockApi({ scenario: 'running', freeze: true });
    const busy = 'a series is loading or a download is running';
    await expect(api.setSelected(0, false)).rejects.toBe(busy);
    await expect(api.setSelectedMany([0], false)).rejects.toBe(busy);
    await expect(api.setAll(false)).rejects.toBe(busy);
    await expect(api.selectRange('1')).rejects.toBe(busy);
    await expect(api.chooseFolder()).rejects.toBe(busy);
    await expect(api.start()).rejects.toBe(busy);
  });

  it('refuses to start while a series loads', async () => {
    const api = createMockApi({ scenario: 'loading', freeze: true });
    await expect(api.start()).rejects.toBe('a series is loading or a download is running');
  });

  it('keeps the bundle flag a run started with', async () => {
    const api = createMockApi('done');
    expect((await api.state()).run.bundle).toBe(false);
    await api.setOptions({ ...(await api.state()).options, packAs: 'bundle' });
    expect((await api.state()).run.bundle).toBe(false);
  });

  it('reports the loaded language and scanlator, apart from the options', async () => {
    const api = createMockApi('loaded');
    await api.setOptions({ ...(await api.state()).options, language: 'es' });
    const state = await api.state();
    expect(state.loadedLanguage).toBe('');
    expect(state.options.language).toBe('es');
    expect(state.loadedScanlator).toBe(state.options.scanlator);
  });

  it('starts finished with a summary for the done scenario, no failures without &fail', async () => {
    const api = createMockApi('done');
    const state = await api.state();
    expect(state.summary).not.toBeNull();
    expect(state.summary!.failures).toHaveLength(0);
    expect(state.summary!.skipped).toBe(0);
    expect(state.running).toBe(false);
  });

  it('starts mid-run with mixed job states including a failure when fail is set', async () => {
    const api = createMockApi({ scenario: 'running', freeze: true, fail: true });
    const state = await api.state();
    expect(state.running).toBe(true);
    const states = new Set(state.jobs.map((j) => j.state));
    expect(states.has('done')).toBe(true);
    expect(states.has('failed')).toBe(true);
    // jobs only exist for started chapters, as in the backend
    expect(state.jobs.length).toBeLessThan(state.run.chapters);
  });

  it('has no failures mid-run without the fail flag', async () => {
    const api = createMockApi({ scenario: 'running', freeze: true, fail: false });
    const state = await api.state();
    expect(state.running).toBe(true);
    expect(state.jobs.every((j) => j.state !== 'failed')).toBe(true);
  });

  it('skips the sixth chapter with skip, and counts it in the summary', async () => {
    const state = await createMockApi({ scenario: 'done', skip: true }).state();
    expect(state.summary!.skipped).toBe(1);
    expect(state.summary!.failures).toHaveLength(0);
    const skipped = state.jobs.filter((j) => j.state === 'skipped');
    expect(skipped).toHaveLength(1);
    expect(skipped[0].number).toBe(603); // the sixth chapter run, 598 being the first
    expect(skipped[0].reason).not.toBe('');
    // the chapter that came out skipped is not saved
    expect(state.summary!.saved).toBe(state.run.chapters - 1);
  });

  it('has no skipped chapter without the skip flag', async () => {
    const state = await createMockApi('done').state();
    expect(state.jobs.some((j) => j.state === 'skipped')).toBe(false);
  });

  it('names a real chapter of the run in the done warning', async () => {
    const log = await createMockApi('done').log();
    expect(log.map((l) => l.text)).toContain('Chapter 599: page 7 is AVIF but could not be converted; kept as is');
  });

  it('runs a bundle as a single job', async () => {
    const state = await createMockApi({ scenario: 'bundle', freeze: true }).state();
    expect(state.run.bundle).toBe(true);
    expect(state.jobs).toHaveLength(1);
    expect(state.jobs[0].status).not.toBe('');
  });

  it('names the bundle after the format it packs as', async () => {
    const cases = [
      ['cbz', 'One Piece 1-3.cbz'],
      ['cbr', 'One Piece 1-3.cbr'],
      ['epub', 'One Piece 1-3.epub'],
      ['raw', 'One Piece 1-3'],
    ] as const;
    for (const [format, name] of cases) {
      const api = createMockApi({ chapters: 40 });
      await api.load('https://example.com/series/a');
      await api.selectRange('1165-1167');
      await api.setOptions({ ...(await api.state()).options, format, packAs: 'bundle' });
      vi.useFakeTimers();
      try {
        await api.start();
        await vi.advanceTimersByTimeAsync(60_000);
        expect((await api.state()).summary?.bundle).toBe(name);
      } finally {
        vi.useRealTimers();
      }
    }
  });

  it('forgets the loaded series on clear, keeping the options and the log', async () => {
    const api = createMockApi({ chapters: 30 });
    await api.load('https://example.com/series/a');
    await api.selectRange('1165-1167');
    await api.setOptions({ ...(await api.state()).options, format: 'raw' });
    const log = await api.log();

    await api.clear();
    const state = await api.state();
    expect(state).toMatchObject({ url: '', loaded: false, title: '', chapterCount: 0, selectedCount: 0, loadError: '' });
    expect(state.languages).toBeNull();
    expect(await api.chapters()).toEqual([]);
    expect(await api.selection()).toEqual([]);
    expect(state.options.format).toBe('raw');
    expect(await api.log()).toEqual(log);
  });

  it("drops the last run's results on clear", async () => {
    const api = createMockApi('done');
    expect((await api.state()).summary).not.toBeNull();

    await api.clear();
    const state = await api.state();
    expect(state.summary).toBeNull();
    expect(state.jobs).toEqual([]);
  });

  it('refuses to clear while a series loads', async () => {
    const api = createMockApi({ chapters: 30 });
    const loading = api.load('https://example.com/series/a');
    await expect(api.clear()).rejects.toBe('a series is loading or a download is running');
    await loading;
    expect((await api.state()).loaded).toBe(true);
  });

  it('has a load error and nothing listed for the error scenario', async () => {
    const state = await createMockApi('error').state();
    expect(state.loaded).toBe(false);
    expect(state.loadError).not.toBe('');
  });

  it('is stale for the stale scenario', async () => {
    const state = await createMockApi('stale').state();
    expect(state.stale).toBe(true);
    expect(state.selectedCount).toBeGreaterThan(0);
  });

  it('lists a site with groups once per group, and tags the copies', async () => {
    const api = createMockApi({ scenario: 'loaded', site: 'groups', chapters: 15 });
    const state = await api.state();
    expect(state.hasGroupDuplicates).toBe(true);
    expect(state.hasDuplicates).toBe(false);

    const chapters = await api.chapters();
    const first = chapters.find((c) => chapters.filter((d) => d.number === c.number).length > 1);
    expect(first).toBeDefined();
    const copies = chapters.filter((c) => c.number === first!.number);
    expect(copies.map((c) => c.group)).toEqual(['Alpha', 'Delta']);
    expect(copies.every((c) => c.language === '')).toBe(true);
    // a release listed once is Alpha's alone, with no copy to tell it apart from
    const single = chapters.filter((c) => chapters.filter((d) => d.number === c.number).length === 1);
    expect(single.length).toBeGreaterThan(0);
    expect(single.every((c) => c.group === 'Alpha')).toBe(true);
  });

  it('does not flag group copies on a site that lists no groups', async () => {
    const api = createMockApi({ scenario: 'loaded', site: 'multi', chapters: 15 });
    const state = await api.state();
    expect(state.hasGroupDuplicates).toBe(false);
    expect((await api.chapters()).every((c) => c.group === '')).toBe(true);
  });

  it('lists some numbers once per language with dupes', async () => {
    const api = createMockApi({ scenario: 'loaded', dupes: true, chapters: 15 });
    const state = await api.state();
    expect(state.hasDuplicates).toBe(true);
    const chapters = await api.chapters();
    // Find chapter numbers listed in both ca and en (multiples of 5 in newest 15)
    const chaptersByNumber = new Map<number, typeof chapters>();
    for (const c of chapters) {
      if (!chaptersByNumber.has(c.number)) {
        chaptersByNumber.set(c.number, []);
      }
      chaptersByNumber.get(c.number)!.push(c);
    }
    const duplicatedChapters = Array.from(chaptersByNumber.values()).filter((cs) => cs.length > 1);
    expect(duplicatedChapters.length).toBeGreaterThan(0);
    // Primary language is Catalan; duplicates are in English
    const languages = duplicatedChapters[0]!.map((c) => c.language).sort();
    expect(languages).toContain('ca');
    expect(languages).toContain('en');
  });

  it('shows version v1.12.0 for update scenario and v0.0.0-mock otherwise', async () => {
    expect((await createMockApi('update').state()).version).toBe('v1.12.0');
    expect((await createMockApi('loaded').state()).version).toBe('v0.0.0-mock');
    expect((await createMockApi('done').state()).version).toBe('v0.0.0-mock');
  });

  it('shows v1.19.0 for all scenarios with shot, v1.20.0 as the update', async () => {
    const update = await createMockApi({ scenario: 'update', shot: true }).state();
    expect(update.version).toBe('v1.19.0');
    expect(update.update?.latest).toBe('v1.20.0');

    const loaded = await createMockApi({ scenario: 'loaded', shot: true }).state();
    expect(loaded.version).toBe('v1.19.0');
    expect(loaded.update).toBeNull();
  });

  it('links the version to its release whenever it reports a release', async () => {
    const tag = (v: string) => `https://github.com/elboletaire/manga-downloader/releases/tag/${v}`;
    expect((await createMockApi({ scenario: 'loaded', shot: true }).state()).releaseURL).toBe(tag('v1.19.0'));
    expect((await createMockApi({ scenario: 'update', shot: true }).state()).releaseURL).toBe(tag('v1.19.0'));
    expect((await createMockApi('update').state()).releaseURL).toBe(tag('v1.12.0'));
    expect((await createMockApi('loaded').state()).releaseURL).toBe('');
  });

  it('keeps the update not one-click with shot, and opens its prompt with prompt', async () => {
    const shot = (await createMockApi({ scenario: 'update', shot: true }).state()).update;
    expect(shot?.oneClick).toBe(false);
    expect(shot?.prompt).toBeUndefined();

    const prompt = (await createMockApi({ scenario: 'update', shot: true, prompt: true }).state()).update;
    expect(prompt).toMatchObject({ latest: 'v1.20.0', oneClick: true, prompt: true });
    expect((await createMockApi({ scenario: 'loaded', prompt: true }).state()).update).toBeNull();
  });

  it('uses the macOS output path with shot and the Linux path without', async () => {
    const withShot = await createMockApi({ shot: true }).state();
    expect(withShot.options.outputDir).toBe('/Users/reader/Downloads/Manga');

    const withoutShot = await createMockApi({ shot: false }).state();
    expect(withoutShot.options.outputDir).toBe('/home/reader/Downloads');
  });

  it('includes the overflow log in update without shot, skips it with shot', async () => {
    const withoutShot = await createMockApi('update');
    const logWithout = await withoutShot.log();
    expect(logWithout.some((l) => l.text.includes('timed out waiting for'))).toBe(true);

    const withShot = await createMockApi({ scenario: 'update', shot: true });
    const logWith = await withShot.log();
    expect(logWith.some((l) => l.text.includes('timed out waiting for'))).toBe(false);
  });

  it('uses real One Piece chapter numbers and Catalan titles', async () => {
    const api = createMockApi({ chapters: 5 });
    await api.load('https://example.com/series/a');
    const chapters = await api.chapters();
    expect(chapters).toHaveLength(5);
    // newest 5 chapters: 1190-1194
    expect(chapters[0].number).toBe(1190);
    expect(chapters[0].title).toBe('Algú de qui se celebra la mort');
    expect(chapters[4].number).toBe(1194);
    expect(chapters[4].title).toBe('La impermanència de totes les coses');
    // newest chapters have no volume yet
    expect(chapters[0].volume).toBe(0);
  });

  it('assigns real volume numbers from the dataset', async () => {
    const api = createMockApi({ chapters: 30 });
    await api.load('https://example.com/series/a');
    const chapters = await api.chapters();
    // newest 30 chapters: 1165-1194; 1165-1166 are vol 114, 1167-1179 are vol 115
    const ch1165 = chapters.find((c) => c.number === 1165);
    expect(ch1165).toBeDefined();
    expect(ch1165!.volume).toBe(114);
    const ch1167 = chapters.find((c) => c.number === 1167);
    expect(ch1167).toBeDefined();
    expect(ch1167!.volume).toBe(115);
    // 1180+ have no volume
    const ch1180 = chapters.find((c) => c.number === 1180);
    expect(ch1180).toBeDefined();
    expect(ch1180!.volume).toBe(0);
  });

  it('does not log the gui.json line in any scenario', async () => {
    for (const scenario of ['loaded', 'running', 'done', 'update', 'volumes'] as const) {
      const api = createMockApi(scenario);
      const log = await api.log();
      expect(log.some((l) => l.text.includes('gui.json'))).toBe(false);
    }
  });
});

describe('mock volumes', () => {
  it("starts the Volumes box empty, and offers the site's volumes", async () => {
    const api = createMockApi({ chapters: 30 });
    await api.load('https://example.com/series/a');
    const { volumes } = await api.state();
    // One Piece chapters are grouped by their real volumes
    expect(volumes.text).toBe('');
    expect(volumes.site).toBeTruthy();
    expect(volumes.listed).toBeGreaterThan(0);
    // newest 30 chapters include vol 114; first in list (ch 1165) has vol 114
    expect((await api.chapters())[0].volume).toBe(114);
  });

  it('takes a Volumes box that parses, and refuses one that does not', async () => {
    const api = createMockApi({ scenario: 'loaded', chapters: 30 });
    await api.setVolumes(' 1-5/6-10 ');
    expect((await api.state()).volumes.text).toBe('1-5/6-10');
    await expect(api.setVolumes('1-5/5-9')).rejects.toBe('volumes 1 and 2 overlap (1-5 and 5-9)');
    expect((await api.state()).volumes.text).toBe('1-5/6-10');
  });

  it('has no site volumes with novolumes, and refuses to start without any', async () => {
    const api = createMockApi({ scenario: 'loaded', novolumes: true });
    expect((await api.state()).volumes).toEqual({ text: '', site: '', listed: 0 });
    await api.setOptions({ ...(await api.state()).options, packAs: 'volumes' });
    await expect(api.start()).rejects.toMatch(/give the volumes as chapter ranges/);
    await api.setVolumes('1-5');
    await api.start();
    expect((await api.state()).run.volumes).toBe(true);
  });

  it('runs one row per volume, plus the chapters outside them', async () => {
    const state = await createMockApi({ scenario: 'volumes-running', freeze: true }).state();
    expect(state.options.packAs).toBe('volumes');
    // First 32 chapters (ch 598–629) span volumes 61 (6), 62 (11), 63 (12) and 64 (3)
    expect(state.run).toMatchObject({ volumes: true, chapters: 32, rows: 4 });
    expect(state.jobs.every((j) => j.volume)).toBe(true);
    expect(state.jobs.map((j) => j.title)).toEqual([
      'Vol. 61 · chapters 598–603',
      'Vol. 62 · chapters 604–614',
      'Vol. 63 · chapters 615–626',
      'Vol. 64 · chapters 627–629',
    ]);
  });

  it('runs one row per chapter with parts, and says the files merge parts', async () => {
    vi.useFakeTimers();
    try {
      const api = createMockApi({ scenario: 'parts' });
      const state = await api.state();
      expect(state.options.packAs).toBe('parts');
      expect(state.title).toBe('One Piece');
      // the parts share their chapter's One Piece title, plus their part
      const chapters = await api.chapters();
      expect(chapters[0]).toMatchObject({ number: 1, title: 'El drac dels llamps Nidhog' });
      expect(chapters[1]).toMatchObject({ number: 2, title: 'Amb honor Part 1' });
      expect(chapters[2]).toMatchObject({ number: 2, title: 'Amb honor Part 2' });
      // chapter 1, both parts of 2-4 and the open part of 19
      expect(state.selectedCount).toBe(8);
      await api.start();
      const running = await api.state();
      expect(running.run).toMatchObject({ volumes: true, chapters: 8, rows: 5 });
      expect(running.jobs.map((j) => j.title)).toEqual([
        'Chapter 2 · 2 parts',
        'Chapter 3 · 2 parts',
        'Chapter 4 · 2 parts',
      ]);
      await vi.advanceTimersByTimeAsync(60_000);
      expect((await api.state()).summary).toMatchObject({ volumes: 3, saved: 8, parts: true });
    } finally {
      vi.useRealTimers();
    }
  });

  it('counts the chapters of saved volumes when the run ends', async () => {
    vi.useFakeTimers();
    try {
      const api = createMockApi({ scenario: 'volumes', chapters: 40 });
      const state = await api.state();
      // Select chapters manually since they are indexed, not numbered
      const allChapters = await api.chapters();
      const indicesToSelect = allChapters.slice(0, 15).map((c) => c.index);
      await api.setAll(false);
      await api.setSelectedMany(indicesToSelect, true);
      await api.start();
      await vi.advanceTimersByTimeAsync(60_000);
      const { summary, running } = await api.state();
      expect(running).toBe(false);
      expect(summary).not.toBeNull();
      // Volumes 113 (1 ch: 1155), 114 (11 ch: 1156–1166), 115 (3 ch: 1167–1169); 0 loose
      expect(summary).toMatchObject({ volumes: 3, saved: 15 });
    } finally {
      vi.useRealTimers();
    }
  });
});

describe('mockSetupFrom', () => {
  it('reads the flags and the chapter count', () => {
    expect(mockSetupFrom('?scenario=bundle&freeze&dupes&chapters=50')).toEqual({
      scenario: 'bundle',
      freeze: true,
      dupes: true,
      chapters: 50,
      site: 'multi',
      novolumes: false,
      parts: false,
      fail: false,
      skip: false,
      locked: false,
      shot: false,
      fetching: false,
      epub: false,
      prompt: false,
      uiLanguage: 'en',
    });
    expect(mockSetupFrom('?novolumes').novolumes).toBe(true);
    expect(mockSetupFrom('?parts').parts).toBe(true);
    expect(mockSetupFrom('?chapters=nope').chapters).toBeGreaterThan(100);
    expect(mockSetupFrom('?fail').fail).toBe(true);
    expect(mockSetupFrom('?skip').skip).toBe(true);
    expect(mockSetupFrom('?locked').locked).toBe(true);
    expect(mockSetupFrom('?shot').shot).toBe(true);
    expect(mockSetupFrom('?prompt').prompt).toBe(true);
    expect(mockSetupFrom('?epub').epub).toBe(true);
  });

  it('starts with EPUB as the format when epub is set', async () => {
    expect((await createMockApi({ scenario: 'loaded', epub: true }).state()).options.format).toBe('epub');
    expect((await createMockApi('loaded').state()).options.format).toBe('cbz');
  });

  it('reads the site, defaulting to multi', () => {
    expect(mockSetupFrom('?site=plus').site).toBe('plus');
    expect(mockSetupFrom('?site=bogus').site).toBe('multi');
  });
});

describe('mock filters', () => {
  it('offers the languages of the site, and none for a site without them', async () => {
    const multi = createMockApi({ scenario: 'loaded', site: 'multi' });
    // multi site: ca, en, es with All; Catalan is the default shown language
    expect((await multi.state()).languages).toEqual({ values: ['ca', 'en', 'es'], all: true, selected: 'ca' });
    expect((await multi.state()).scanlators).toBeNull();

    const none = createMockApi({ scenario: 'loaded', site: 'none' });
    expect((await none.state()).languages).toBeNull();
    expect((await none.state()).scanlators).toBeNull();
  });

  it('shows the default edition of a site with no All, and every group of a site that lists groups', async () => {
    const plus = createMockApi({ scenario: 'loaded', site: 'plus' });
    expect((await plus.state()).languages).toEqual({ values: ['en', 'es'], all: false, selected: 'en' });

    const groups = createMockApi({ scenario: 'loaded', site: 'groups' });
    expect((await groups.state()).scanlators).toEqual({ values: ['Alpha', 'Delta'], all: true, selected: 'all' });
  });

  it('lists no filters before a load', async () => {
    const api = createMockApi({ scenario: 'empty', site: 'multi' });
    expect((await api.state()).languages).toBeNull();
  });

  it('falls back to all on a language the series lacks, and logs it', async () => {
    const api = createMockApi({ scenario: 'loaded', site: 'multi' });
    await api.setOptions({ ...(await api.state()).options, language: 'xx' });
    await api.load((await api.state()).url);

    const state = await api.state();
    expect(state.options.language).toBe('all');
    expect(state.loadedLanguage).toBe('all');
    expect(state.stale).toBe(false);
    expect((await api.log()).map((l) => l.text)).toContain(
      "language \"xx\" isn't in this series, loading all of them instead",
    );
  });

  it('falls back to the edition the URL points at where the site has no all, and names it', async () => {
    const api = createMockApi({ scenario: 'loaded', site: 'plus' });
    await api.setOptions({ ...(await api.state()).options, language: 'xx' });
    await api.load((await api.state()).url);

    expect((await api.state()).options.language).toBe('en');
    expect((await api.state()).languages?.selected).toBe('en');
    expect((await api.log()).map((l) => l.text)).toContain('language "xx" isn\'t in this series, loading "en" instead');
  });

  it('matches a typed language ignoring case, and takes the listed spelling', async () => {
    const api = createMockApi({ scenario: 'loaded', site: 'multi' });
    await api.setOptions({ ...(await api.state()).options, language: 'ES' });
    await api.load((await api.state()).url);

    expect((await api.state()).options.language).toBe('es');
    expect((await api.state()).loadedLanguage).toBe('es');
    expect((await api.log()).map((l) => l.text)).not.toContain(
      'language "ES" isn\'t in this series, loading all of them instead',
    );
  });

  it('is not stale while a reload started by a filter change runs', async () => {
    const api = createMockApi({ scenario: 'loaded', site: 'multi' });
    await api.setOptions({ ...(await api.state()).options, language: 'es' });
    expect((await api.state()).stale).toBe(true);

    const reload = api.load((await api.state()).url);
    expect((await api.state()).loading).toBe(true);
    expect((await api.state()).stale).toBe(false);
    await reload;
    expect((await api.state()).stale).toBe(false);
  });
});

describe('mock locked chapters', () => {
  it('lists none by default, and every 50th chapter with &locked', async () => {
    const plain = await createMockApi({ scenario: 'loaded' }).chapters();
    expect(plain.some((c) => c.locked)).toBe(false);

    const locked = (await createMockApi({ scenario: 'loaded', locked: true }).chapters()).filter((c) => c.locked);
    expect(locked.length).toBeGreaterThan(0);
    expect(locked.every((c) => c.number % 50 === 0)).toBe(true);
  });

  it('never selects a locked chapter, by click, span, All or range', async () => {
    const api = createMockApi({ scenario: 'empty', locked: true });
    await api.load('https://example.com/manga/one-piece');
    const list = await api.chapters();
    const lockedIdx = list.filter((c) => c.locked).map((c) => c.index);
    expect(lockedIdx.length).toBeGreaterThan(0);

    await api.setSelected(lockedIdx[0], true);
    await api.setSelectedMany(lockedIdx, true);
    expect(await api.selection()).toEqual([]);

    await api.setAll(true);
    expect((await api.selection()).length).toBe(list.length - lockedIdx.length);
    expect((await api.selection()).some((i) => lockedIdx.includes(i))).toBe(false);

    await api.setAll(false);
    const first = list[0].number;
    const n = await api.selectRange(`${first}-${list[list.length - 1].number}`);
    expect(n).toBe(list.length - lockedIdx.length);
  });
});

describe('one-click update', () => {
  it('is offered in the update scenario, but not in the screenshot', async () => {
    expect((await createMockApi('update').state()).update?.oneClick).toBe(true);
    expect((await createMockApi({ scenario: 'update', shot: true }).state()).update?.oneClick).toBe(false);
  });
});

describe('mock fetching rows', () => {
  it('starts each chapter fetching its pages, then downloading them', async () => {
    const api = createMockApi({ scenario: 'running', fetching: true, freeze: true });
    const rows = (await api.state()).jobs;
    expect(rows.some((j) => j.state === 'fetching' && j.total === 0 && j.status !== '')).toBe(true);
    expect(rows.some((j) => j.state === 'downloading')).toBe(true);
  });

  it('keeps the other running scenarios free of fetching rows', async () => {
    const rows = (await createMockApi({ scenario: 'running', freeze: true }).state()).jobs;
    expect(rows.some((j) => j.state === 'fetching')).toBe(false);
  });
});

describe('mockSetupFrom language', () => {
  it('starts in English unless ?lang picks a language, and ?lang=system follows the browser', () => {
    expect(mockSetupFrom('?mock').uiLanguage).toBe('en');
    expect(mockSetupFrom('?mock&lang=es').uiLanguage).toBe('es');
    expect(mockSetupFrom('?lang=ca').uiLanguage).toBe('ca');
    expect(mockSetupFrom('?lang=system').uiLanguage).toBe('');
    expect(mockSetupFrom('?lang=fr').uiLanguage).toBe('en');
  });
});

describe('mock app preferences', () => {
  it('starts with the language it was set up with', async () => {
    expect((await createMockApi('empty').state()).prefs).toEqual({ uiLanguage: 'en', checkUpdates: true });
    expect((await createMockApi({ uiLanguage: 'es' }).state()).prefs.uiLanguage).toBe('es');
  });

  it('keeps the language and pushes it', async () => {
    const api = createMockApi('empty');
    const pushed: State[] = [];
    const stop = api.onState((s) => pushed.push(s));
    await api.setUILanguage('ca');
    expect(pushed.at(-1)?.prefs.uiLanguage).toBe('ca');
    expect((await api.state()).prefs.uiLanguage).toBe('ca');
    await api.setUILanguage('');
    expect((await api.state()).prefs.uiLanguage).toBeUndefined();
    stop();
  });

  it('turning updates off clears the update and pushes that, and on brings the check back', async () => {
    const api = createMockApi('update');
    expect((await api.state()).update).not.toBeNull();
    const pushed: State[] = [];
    const stop = api.onState((s) => pushed.push(s));
    await api.setCheckUpdates(false);
    expect(pushed.at(-1)?.prefs.checkUpdates).toBe(false);
    expect(pushed.at(-1)?.update).toBeNull();
    await api.setCheckUpdates(true);
    expect((await api.state()).update).not.toBeNull();
    stop();
  });
});
