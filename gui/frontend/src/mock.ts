// A stand-in for the Go App, for working on the UI in a plain browser
// (pnpm dev) and in tests. It keeps its own state, simulates a load and a
// download, and pushes the same events the Go side does.
//
// Add ?mock to the URL to use it inside the app too. ?scenario= starts it in
// a given state, so screenshots need no clicking: empty, loading, error,
// loaded, stale, running, bundle, cancelling, done or update. update is
// loaded too, as a release build (v1.12.0) with a newer release (v1.13.0): the
// status bar shows the yellow "!". &freeze stops the simulated run from moving
// (stable screenshots), &dupes lists some numbers twice, once per language, and
// &chapters=N sets the list's length.
//
// &site= picks what the series lists for the two filters: multi (the default:
// languages en, es, ja with All, no groups), plus (Manga Plus: languages
// without All, the URL's edition shown by default), groups (Atsumaru: groups
// with All, the largest shown by default, no languages), or none (both fields
// disabled). A language or group the series doesn't list falls back on load,
// as the backend does: typed "xx" under multi loads "all", under plus the
// edition the URL points at. Typed values match ignoring case, and take the
// spelling the series lists.

import type { Api, Unsubscribe } from './api';
import type { Chapter, FilterChoice, Job, LogLine, Options, State, Summary, Update } from './types';

const CHAPTERS = 312;
const PAGES = 12;
const TICK_MS = 160;

const SCENARIOS = [
  'empty',
  'loading',
  'error',
  'loaded',
  'stale',
  'running',
  'bundle',
  'cancelling',
  'done',
  'update',
] as const;

export type Scenario = (typeof SCENARIOS)[number];

const SITES = ['multi', 'plus', 'groups', 'none'] as const;

export type MockSite = (typeof SITES)[number];

export interface MockSetup {
  scenario: Scenario;
  /** The simulated run doesn't advance */
  freeze: boolean;
  /** Some numbers are listed twice, in two languages */
  dupes: boolean;
  chapters: number;
  site: MockSite;
}

/** What the mock series offers for each filter; null when the site lists none */
interface MockOffer {
  values: string[];
  all: boolean;
  /** The value shown when no filter is set, for the sites that show one */
  current: string;
}

interface MockOffers {
  language: MockOffer | null;
  scanlator: MockOffer | null;
}

const OFFERS: Record<MockSite, MockOffers> = {
  multi: { language: { values: ['en', 'es', 'ja'], all: true, current: '' }, scanlator: null },
  plus: { language: { values: ['en', 'es'], all: false, current: 'en' }, scanlator: null },
  groups: { language: null, scanlator: { values: ['Alpha', 'Delta'], all: true, current: 'Delta' } },
  none: { language: null, scanlator: null },
};

const REPO = 'https://github.com/elboletaire/manga-downloader';
const RELEASE_VERSION = 'v1.12.0';

/** The newer release the update scenario reports; the others are dev builds */
const UPDATES: Partial<Record<Scenario, Update>> = {
  update: { latest: 'v1.13.0', url: `${REPO}/releases/latest` },
};

function siteFrom(search: string): MockSite {
  const value = new URLSearchParams(search).get('site');
  return (SITES as readonly string[]).includes(value ?? '') ? (value as MockSite) : 'multi';
}

function scenarioFrom(search: string): Scenario {
  const value = new URLSearchParams(search).get('scenario');
  return (SCENARIOS as readonly string[]).includes(value ?? '') ? (value as Scenario) : 'empty';
}

/** The mock's whole setup from the page's query string */
export function mockSetupFrom(search: string): MockSetup {
  const params = new URLSearchParams(search);
  const n = Number(params.get('chapters'));
  return {
    scenario: scenarioFrom(search),
    freeze: params.has('freeze'),
    dupes: params.has('dupes'),
    chapters: Number.isInteger(n) && n > 0 ? n : CHAPTERS,
    site: siteFrom(search),
  };
}

function defaultOptions(): Options {
  return {
    outputDir: '/home/reader/Downloads',
    format: 'cbz',
    bundle: false,
    language: '',
    scanlator: '',
    convertImages: 'avif',
    skipPages: '',
    filenameTemplate: '{{.Series}} {{.Number}} - {{.Title}}{{if gt .Version 1}} v{{.Version}}{{end}}',
    browserVisible: false,
  };
}

// Chapter names, so the list looks like a real series rather than a counter
const NAMES = [
  'The Lighthouse Keeper',
  'Salt and Ink',
  'A Door in the Tide',
  'Paper Lanterns',
  'The Long Way Round',
  'Inheritance',
  'What the River Took',
  'Night Market',
  'Second Wind',
  'Cartographers',
  'The Quiet Hour',
  'Ashfall',
];

function mockChapters(count: number, withLanguages: boolean): Chapter[] {
  const list: Chapter[] = [];
  const add = (number: number, title: string, language: string) =>
    list.push({ index: list.length, number, title, language });

  for (let i = 1; i <= count; i++) {
    // every seventh chapter has a name, as sites often only name some
    const title = i % 7 === 1 ? `Chapter ${i}: ${NAMES[Math.floor(i / 7) % NAMES.length]}` : `Chapter ${i}`;
    add(i, title, withLanguages ? 'en' : '');
    if (withLanguages && i % 5 === 0) {
      add(i, `Capítulo ${i}`, 'es');
    }
    if (i % 40 === 0) {
      add(i + 0.5, `Chapter ${i}.5: Extra`, withLanguages ? 'en' : '');
    }
  }
  return list;
}

type Listener<T> = (v: T) => void;

class Bus<T> {
  private listeners = new Set<Listener<T>>();
  subscribe(cb: Listener<T>): Unsubscribe {
    this.listeners.add(cb);
    return () => {
      this.listeners.delete(cb);
    };
  }
  emit(v: T): void {
    for (const cb of this.listeners) {
      cb(v);
    }
  }
}

export function createMockApi(setup: Scenario | Partial<MockSetup> = 'empty'): Api {
  const {
    scenario,
    freeze,
    dupes,
    chapters: chapterCount,
    site,
  } = {
    scenario: 'empty' as Scenario,
    freeze: false,
    dupes: false,
    chapters: CHAPTERS,
    site: 'multi' as MockSite,
    ...(typeof setup === 'string' ? { scenario: setup } : setup),
  };
  const offers = OFFERS[site];
  const options = defaultOptions();
  let url = '';
  let loading = false;
  let loadError = '';
  let loaded = false;
  let title = '';
  let chapters: Chapter[] = [];
  let selected = new Set<number>();
  let applied = { language: '', scanlator: '' };
  // whether a load has been attempted, so the filters are the series' own choices
  let identified = false;
  let running = false;
  let cancelling = false;
  let runChapters = 0;
  // the bundle option as the run started with it, as the backend's RunInfo
  let runBundle = false;
  let jobs: Job[] = [];
  let summary: Summary | null = null;
  let log: LogLine[] = [];
  let timer: ReturnType<typeof setInterval> | undefined;

  const stateBus = new Bus<State>();
  const chaptersBus = new Bus<Chapter[]>();
  const selectionBus = new Bus<number[]>();
  const logBus = new Bus<LogLine[]>();

  const selectionOf = () => [...selected].sort((a, b) => a - b);
  const pushLog = (level: LogLine['level'], text: string) => {
    log = [...log, { level, text }].slice(-500);
    logBus.emit(log);
  };
  // what a filter's selector shows: the value the list was loaded with, else
  // the site's default (its current value, else every value where it takes all)
  const filterOf = (kind: 'language' | 'scanlator'): FilterChoice | null => {
    const offer = offers[kind];
    if (!identified || offer === null) {
      return null;
    }
    const loadedWith = applied[kind];
    let selected = loadedWith;
    if (loadedWith === '') {
      selected = offer.current !== '' ? offer.current : offer.all ? 'all' : '';
    }
    return { values: [...offer.values], all: offer.all, selected };
  };
  // a typed value the series doesn't offer falls back as the backend does: to
  // "all" where the site takes it, else to the site's current value, else to no
  // filter. A value that matches one ignoring case takes the listed spelling.
  const fallbackFor = (kind: 'language' | 'scanlator', label: string): void => {
    const offer = offers[kind];
    const typed = options[kind];
    if (offer === null || typed === '' || typed === 'all') {
      return;
    }
    const listed = offer.values.find((v) => v.toLowerCase() === typed.toLowerCase());
    if (listed !== undefined) {
      options[kind] = listed;
      return;
    }
    const fallback = offer.all ? 'all' : offer.current;
    options[kind] = fallback;
    const shown = fallback === 'all' ? 'all of them' : fallback === '' ? 'no filter' : `"${fallback}"`;
    pushLog('info', `${label} "${typed}" isn't in this series, loading ${shown} instead`);
  };
  const snapshot = (): State => ({
    url,
    loading,
    loaded,
    loadError,
    title,
    chapterCount: chapters.length,
    hasDuplicates: chapters.some((c) => c.language !== ''),
    stale: loaded && !loading && (options.language !== applied.language || options.scanlator !== applied.scanlator),
    loadedLanguage: applied.language,
    loadedScanlator: applied.scanlator,
    languages: filterOf('language'),
    scanlators: filterOf('scanlator'),
    selectedCount: selected.size,
    running,
    cancelling,
    run: { chapters: runChapters, bundle: runBundle },
    jobs: jobs.map((j) => ({ ...j })),
    summary: summary ? { ...summary, failures: [...summary.failures] } : null,
    options: { ...options },
    version: UPDATES[scenario] ? RELEASE_VERSION : 'v0.0.0-mock',
    releaseURL: UPDATES[scenario] ? `${REPO}/releases/tag/${RELEASE_VERSION}` : '',
    update: UPDATES[scenario] ?? null,
  });
  const push = () => stateBus.emit(snapshot());

  function finishLoad(): void {
    loading = false;
    loaded = true;
    identified = true;
    fallbackFor('language', 'language');
    fallbackFor('scanlator', 'scanlation group');
    title = 'The Cartographer of Lost Harbours';
    chapters = mockChapters(chapterCount, dupes || url.includes('dupes') || options.language === 'all');
    applied = { language: options.language, scanlator: options.scanlator };
    selected = new Set<number>();
    jobs = [];
    summary = null;
    pushLog('info', `Loaded ${title}: ${chapters.length} chapters`);
    chaptersBus.emit(chapters);
    selectionBus.emit([]);
    push();
  }

  function failLoad(): void {
    loading = false;
    loaded = false;
    identified = true;
    applied = { language: options.language, scanlator: options.scanlator };
    chapters = [];
    selected = new Set<number>();
    loadError = "Couldn't fetch the chapter list: unexpected status 403 Forbidden";
    pushLog('error', loadError);
    chaptersBus.emit([]);
    selectionBus.emit([]);
    push();
  }

  function finishRun(): void {
    clearInterval(timer);
    timer = undefined;
    running = false;
    const saved = jobs.filter((j) => j.state === 'done').length;
    const failures = jobs.filter((j) => j.state === 'failed').map((j) => `${j.title}: ${j.reason}`);
    const skipped = jobs.filter((j) => j.state === 'skipped').length;
    const bundled = runBundle;
    summary = {
      dir: options.outputDir,
      cancelled: cancelling,
      saved: bundled && failures.length === 0 && !cancelling ? runChapters : saved,
      bundled,
      bundle: bundled && !cancelling ? `${title} 1-${runChapters}.cbz` : '',
      skipped,
      failures,
      error: '',
    };
    pushLog('info', cancelling ? `Cancelled: ${saved} saved` : `Finished: ${saved} saved`);
    if (failures.length > 0) {
      pushLog('warn', `${failures.length} chapter(s) failed`);
    }
    push();
  }

  // the chapters of the run that haven't started; the backend only creates a
  // job when a chapter starts, so the mock does too
  let pending: Chapter[] = [];

  /** One step of the simulated run. Returns false once it's over. */
  function step(): boolean {
    if (runBundle) {
      const j = jobs[0];
      const done = Math.min(j.total, j.done + 4);
      const fetched = done < j.total / 2;
      jobs = [
        {
          ...j,
          done,
          state: done >= j.total ? 'done' : fetched ? 'downloading' : 'archiving',
          status:
            done >= j.total
              ? ''
              : fetched
                ? `Downloading chapter ${Math.floor((done / j.total) * 2 * runChapters) + 1}...`
                : 'Creating the bundle...',
        },
      ];
      return done < j.total;
    }

    jobs = jobs.map((j) => {
      if (j.state !== 'downloading' && j.state !== 'archiving') {
        return j;
      }
      if (j.number === 4 && j.done >= PAGES / 2) {
        return { ...j, state: 'failed', reason: 'download: page 5 returned 404 Not Found' };
      }
      // uneven speeds, so the rows don't move in lockstep
      const done = Math.min(j.total, j.done + 1 + ((j.number * 7) % 4));
      const state = done >= j.total ? 'done' : done >= j.total / 2 ? 'archiving' : 'downloading';
      return { ...j, done, state };
    });

    const active = jobs.filter((j) => j.state === 'downloading' || j.state === 'archiving').length;
    if (!cancelling && active < 3 && pending.length > 0) {
      const c = pending.shift()!;
      if (c.number === 6) {
        jobs = [
          ...jobs,
          {
            title: c.title,
            number: c.number,
            total: 0,
            done: 0,
            state: 'skipped',
            reason: 'every page was skipped',
            status: '',
          },
        ];
      } else {
        jobs = [
          ...jobs,
          { title: c.title, number: c.number, total: PAGES * 2, done: 0, state: 'downloading', reason: '', status: '' },
        ];
      }
    }

    return (
      jobs.some((j) => j.state === 'downloading' || j.state === 'archiving') || (!cancelling && pending.length > 0)
    );
  }

  function beginRun(list: Chapter[]): void {
    running = true;
    cancelling = false;
    runChapters = list.length;
    runBundle = options.bundle;
    summary = null;
    pending = [...list];
    jobs = runBundle
      ? [
          {
            title: `Bundle: chapters 1-${list.length}`,
            number: 0,
            total: list.length * PAGES * 2,
            done: 0,
            state: 'downloading',
            reason: '',
            status: 'Gathering info...',
          },
        ]
      : [];
  }

  function tick(): void {
    if (!step()) {
      finishRun();
      return;
    }
    push();
  }

  function simulateRun(list: Chapter[]): void {
    beginRun(list);
    push();
    if (!freeze) {
      timer = setInterval(tick, TICK_MS);
    }
  }

  if (scenario === 'loading') {
    url = 'https://example.com/series/the-cartographer';
    loading = true;
    if (!freeze) {
      setTimeout(finishLoad, 2500);
    }
  } else if (scenario === 'error') {
    url = 'https://unknown.example.org/manga/some-series';
    failLoad();
    log = [];
    pushLog('info', `Loading ${url}`);
    pushLog('error', loadError);
  } else if (scenario !== 'empty') {
    url = 'https://example.com/series/the-cartographer';
    finishLoad();
    pushLog('info', 'Using the saved options from gui.json');
    for (const c of chapters.slice(0, 12)) {
      selected.add(c.index);
    }
    for (const c of chapters.slice(40, 52)) {
      selected.add(c.index);
    }
    selectionBus.emit(selectionOf());
  }
  if (scenario === 'stale') {
    options.language = 'es';
  }
  if (scenario === 'running' || scenario === 'cancelling' || scenario === 'bundle' || scenario === 'done') {
    options.bundle = scenario === 'bundle';
    const picked = chapters.filter((c) => selected.has(c.index)).slice(0, 12);
    pushLog('info', `Downloading ${picked.length} chapter(s) of ${title}`);
    beginRun(picked);
    // run the simulation forward to an interesting point
    const steps = scenario === 'done' ? Infinity : scenario === 'bundle' ? 8 : 14;
    for (let i = 0; i < steps && step(); i++) {
      // advancing
    }
    if (scenario === 'cancelling') {
      cancelling = true;
      pushLog('info', 'Cancelling: chapters already downloading will finish, no new ones start');
    }
    if (scenario === 'done') {
      pushLog('warn', 'Chapter 2: page 7 is AVIF but could not be converted; kept as is');
      finishRun();
    } else if (!freeze) {
      timer = setInterval(tick, TICK_MS);
    }
  }
  push();

  const api: Api = {
    async state() {
      return snapshot();
    },
    async chapters() {
      return chapters.map((c) => ({ ...c }));
    },
    async selection() {
      return selectionOf();
    },
    async log() {
      return log.slice();
    },

    async load(next) {
      const trimmed = next.trim();
      if (trimmed === '') {
        throw 'enter the URL of a series first';
      }
      if (loading || running) {
        throw 'a series is loading or a download is running';
      }
      url = trimmed;
      loadError = '';
      loading = true;
      push();
      await new Promise((resolve) => setTimeout(resolve, 400));
      if (trimmed.includes('unknown')) {
        failLoad();
        return;
      }
      finishLoad();
    },
    async setSelected(index, on) {
      if (loading || running) {
        throw 'a series is loading or a download is running';
      }
      if (index < 0 || index >= chapters.length) {
        return;
      }
      if (on) {
        selected.add(index);
      } else {
        selected.delete(index);
      }
      selectionBus.emit(selectionOf());
      push();
    },
    async setSelectedMany(indices, on) {
      if (loading || running) {
        throw 'a series is loading or a download is running';
      }
      if (indices.some((index) => index < 0 || index >= chapters.length)) {
        throw `chapter ${indices.find((index) => index < 0 || index >= chapters.length)} is not in the list`;
      }
      for (const index of indices) {
        if (on) {
          selected.add(index);
        } else {
          selected.delete(index);
        }
      }
      selectionBus.emit(selectionOf());
      push();
    },
    async setAll(on) {
      if (loading || running) {
        throw 'a series is loading or a download is running';
      }
      selected = new Set(on ? chapters.map((c) => c.index) : []);
      selectionBus.emit(selectionOf());
      push();
    },
    async selectRange(text) {
      if (loading || running) {
        throw 'a series is loading or a download is running';
      }
      const matched: number[] = [];
      for (const part of text
        .split(',')
        .map((p) => p.trim())
        .filter(Boolean)) {
        const [a, b] = part.split('-').map(Number);
        if (Number.isNaN(a)) {
          throw `invalid range: ${part}`;
        }
        const hi = b ?? a;
        for (const c of chapters) {
          if (c.number >= a && c.number <= hi) {
            matched.push(c.index);
          }
        }
      }
      for (const i of matched) {
        selected.add(i);
      }
      selectionBus.emit(selectionOf());
      push();
      return matched.length;
    },
    async setOptions(next) {
      if (loading || running) {
        throw 'a series is loading or a download is running';
      }
      Object.assign(options, next);
      push();
    },
    async chooseFolder() {
      if (loading || running) {
        throw 'a series is loading or a download is running';
      }
      const picked = window.prompt('Output folder (mock dialog)', options.outputDir) ?? '';
      if (picked !== '') {
        options.outputDir = picked;
        push();
      }
      return picked;
    },
    async openFolder() {
      pushLog('info', `would open ${options.outputDir} (mock)`);
    },
    openURL(url) {
      window.open(url, '_blank', 'noopener');
    },
    async start() {
      if (loading || running) {
        throw 'a series is loading or a download is running';
      }
      if (!loaded) {
        throw 'load a series first';
      }
      if (options.language !== applied.language || options.scanlator !== applied.scanlator) {
        throw 'the language or scanlator changed: reload the series first';
      }
      const picked = chapters.filter((c) => selected.has(c.index));
      if (picked.length === 0) {
        throw 'select at least one chapter';
      }
      pushLog('info', `Downloading ${picked.length} chapter(s) of ${title}`);
      simulateRun(picked);
    },
    async cancel() {
      if (!running || cancelling) {
        return;
      }
      cancelling = true;
      pushLog('info', 'Cancelling: chapters already downloading will finish, no new ones start');
      push();
    },

    onState: (cb) => stateBus.subscribe(cb),
    onChapters: (cb) => chaptersBus.subscribe(cb),
    onSelection: (cb) => selectionBus.subscribe(cb),
    onLog: (cb) => logBus.subscribe(cb),
  };

  return api;
}

export { scenarioFrom };
