// A stand-in for the Go App, for working on the UI in a plain browser
// (pnpm dev) and in tests. It keeps its own state, simulates a load and a
// download, and pushes the same events the Go side does.
//
// Add ?mock to the URL to use it inside the app too. ?scenario=loaded,
// running or done starts it in that state, so screenshots need no clicking.

import type { Api, Unsubscribe } from './api';
import type { Chapter, Job, LogLine, Options, State, Summary } from './types';

const CHAPTERS = 30;
const PAGES = 12;
const TICK_MS = 120;

type Scenario = 'empty' | 'loaded' | 'running' | 'done';

function scenarioFrom(search: string): Scenario {
  const value = new URLSearchParams(search).get('scenario');
  return value === 'loaded' || value === 'running' || value === 'done' ? value : 'empty';
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
    filenameTemplate: '{{.Title}} - Chapter {{.Number}}',
    browserVisible: false,
  };
}

function mockChapters(withLanguages: boolean): Chapter[] {
  const list: Chapter[] = [];
  for (let i = 1; i <= CHAPTERS; i++) {
    list.push({
      index: i - 1,
      number: i,
      title: `Chapter ${i}`,
      language: withLanguages ? 'eng' : '',
    });
  }
  if (withLanguages) {
    list.push({ index: CHAPTERS, number: 12, title: 'Chapter 12', language: 'esp' });
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

export function createMockApi(scenario: Scenario = 'empty'): Api {
  const options = defaultOptions();
  let url = '';
  let loading = false;
  let loadError = '';
  let loaded = false;
  let title = '';
  let chapters: Chapter[] = [];
  let selected = new Set<number>();
  let applied = { language: '', scanlator: '' };
  let running = false;
  let cancelling = false;
  let runChapters = 0;
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
  const snapshot = (): State => ({
    url,
    loading,
    loaded,
    loadError,
    title,
    chapterCount: chapters.length,
    hasDuplicates: chapters.some((c) => c.language !== ''),
    stale: loaded && (options.language !== applied.language || options.scanlator !== applied.scanlator),
    selectedCount: selected.size,
    running,
    cancelling,
    run: { chapters: runChapters, bundle: options.bundle },
    jobs: jobs.map((j) => ({ ...j })),
    summary: summary ? { ...summary, failures: [...summary.failures] } : null,
    options: { ...options },
    version: 'v0.0.0-mock',
  });
  const push = () => stateBus.emit(snapshot());

  function finishLoad(): void {
    loading = false;
    loaded = true;
    title = 'Mock Series';
    chapters = mockChapters(url.includes('dupes') || options.language === 'all');
    applied = { language: options.language, scanlator: options.scanlator };
    selected = new Set<number>();
    jobs = [];
    summary = null;
    pushLog('info', `Loaded ${title}: ${chapters.length} chapters`);
    chaptersBus.emit(chapters);
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
    summary = {
      dir: options.outputDir,
      cancelled: cancelling,
      saved,
      bundled: false,
      bundle: '',
      skipped,
      failures,
      error: '',
    };
    pushLog('info', `Finished: ${saved} saved`);
    push();
  }

  function simulateRun(list: Chapter[]): void {
    running = true;
    cancelling = false;
    runChapters = list.length;
    summary = null;
    // the mock fails the fourth chapter (see the tick below) so the failure
    // path is visible
    jobs = list.map((c) => ({
      title: c.title,
      number: c.number,
      total: PAGES * 2,
      done: 0,
      state: 'downloading',
      reason: '',
      status: '',
    }));
    push();

    timer = setInterval(() => {
      let moving = false;
      jobs = jobs.map((j, i) => {
        if (j.state === 'done' || j.state === 'failed' || j.state === 'skipped') {
          return j;
        }
        moving = true;
        if (i === 3 && j.done >= PAGES / 2) {
          return { ...j, state: 'failed', reason: 'download: page 5 returned 404' };
        }
        const done = Math.min(j.total, j.done + 3);
        const state = done >= j.total ? 'done' : done >= j.total / 2 ? 'archiving' : 'downloading';
        return { ...j, done, state };
      });
      if (!moving) {
        finishRun();
        return;
      }
      push();
    }, TICK_MS);
  }

  if (scenario !== 'empty') {
    url = 'https://example.com/series/mock-series';
    finishLoad();
    for (const i of [0, 1, 2, 3, 4, 5, 6, 7, 8, 9]) {
      selected.add(i);
    }
    selectionBus.emit(selectionOf());
    push();
  }
  if (scenario === 'running') {
    simulateRun(chapters.filter((_, i) => selected.has(i)).slice(0, 8));
  }
  if (scenario === 'done') {
    // a finished run, built without the timer
    const picked = chapters.filter((_, i) => selected.has(i)).slice(0, 8);
    runChapters = picked.length;
    jobs = picked.map((c, i) => ({
      title: c.title,
      number: c.number,
      total: PAGES * 2,
      done: PAGES * 2,
      state: i === 3 ? 'failed' : 'done',
      reason: i === 3 ? 'download: page 5 returned 404' : '',
      status: '',
    }));
    summary = {
      dir: options.outputDir,
      cancelled: false,
      saved: 7,
      bundled: false,
      bundle: '',
      skipped: 0,
      failures: ['Chapter 4: download: page 5 returned 404'],
      error: '',
    };
  }

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
        loading = false;
        loaded = false;
        chapters = [];
        loadError = 'site not recognised';
        pushLog('error', 'site not recognised');
        push();
        return;
      }
      finishLoad();
    },
    async setSelected(index, on) {
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
    async setAll(on) {
      selected = new Set(on ? chapters.map((c) => c.index) : []);
      selectionBus.emit(selectionOf());
      push();
    },
    async selectRange(text) {
      const matched: number[] = [];
      for (const part of text.split(',').map((p) => p.trim()).filter(Boolean)) {
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
    async start() {
      if (!loaded) {
        throw 'load a series first';
      }
      if (options.language !== applied.language || options.scanlator !== applied.scanlator) {
        throw 'the language or scanlator changed: reload the series first';
      }
      if (running) {
        throw 'a series is loading or a download is running';
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
