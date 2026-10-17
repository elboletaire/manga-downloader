// A stand-in for the Go App, for working on the UI in a plain browser
// (pnpm dev) and in tests. It keeps its own state, simulates a load and a
// download, and pushes the same events the Go side does.
//
// Add ?mock to the URL to use it inside the app too. ?scenario= starts it in
// a given state, so screenshots need no clicking: empty, loading, error,
// loaded, stale, running, bundle, cancelling, done, update, check, volumes
// (loaded, packing by volume, with three volumes and a half selected) or
// volumes-running (a run by volume in progress) or parts (loaded, packing each
// chapter with its parts merged, as &parts lists them). update is loaded too, as a
// release build (v1.17.0 with &shot, v1.12.0 without) with a newer release
// (v1.18.0 with &shot, v1.13.0 without): the status bar shows the yellow "!".
// check is loading a site that needs the browser window, with the alert asking
// the user to look at it; loading a URL with "check" in it does the same until
// the load ends. &freeze stops the simulated run from moving (stable
// screenshots), &dupes lists some numbers twice in two languages, &chapters=N
// shows the newest N chapters, &parts lists a series whose chapters come in
// two parts (a real listing, its later chapters locked), &novolumes lists no volumes (the series
// otherwise files its chapters by their real One Piece volume), &locked marks
// some chapters locked (see below), &fail makes
// the fourth chapter fail halfway and &skip makes the sixth one come out
// skipped, every page excluded by --skip-pages (both off by default; every run
// saves every chapter without them), &fetching has each chapter's row spend its
// first few ticks fetching its page list (frozen, the newest rows are still
// fetching), &epub starts the output as EPUB, &prompt
// (with update) makes the update one-click and opens its prompt on load, &shot
// uses production release versions and a macOS output path (for screenshots),
// and VITE_MOCK=1 at build time includes the mock in production builds (e.g.
// for the screenshot capture build; set via Wails frontend env). ?lang=en|es|ca
// starts the window in that language, ?lang=system follows the browser's, and
// without it the window is English, so screenshots and dev stay English. Under
// multi the language filter shows that language and the list is in it, titled
// from OP_TITLES; every fifth chapter also appears once in another language.
//
// &site= picks what the series lists for the two filters: multi (the default:
// languages ca, en, es with All, the window's language shown by default, no groups), plus
// (Manga Plus: languages without All, the URL edition shown by default),
// groups (groups with All, every group shown by default, each chapter listed
// once per group, no languages), or none (both fields disabled: a typed value shows cleared, as
// the site ignores it, and is kept for the sites that take it). A language or
// group the series doesn't list falls back on load, as the backend does: typed
// "xx" under multi loads "all", under plus the edition the URL points at.
// Typed values match ignoring case, and take the spelling the series lists.
// &locked lists every 50th chapter as locked: the list shows its lock
// and never selects it, as the backend does.

import type { Api, Unsubscribe } from './api';
import { createT, LANGS, resolveLanguage, type Lang, type LangPref, type Vars } from './i18n';
import type { Chapter, FilterChoice, Job, LogLine, Message, Options, Prefs, State, Summary, Update } from './types';
import { groupChapters, groupParts, listedVolumes, parseVolumes, siteVolumes } from './volumes';
import { OP_TITLES } from './mock-onepiece';

const TEMPLATE_DEFAULT =
  '{{.Series}} {{.Number}} - {{.Title}}{{if .Group}} [{{.Group}}]{{end}}{{if gt .Version 1}} v{{.Version}}{{end}}';
const TEMPLATE_WARNING =
  'This template has no {{.Number}}, {{.Title}}, {{.SiteTitle}} or {{.Volume}}, so files only differ by a version suffix.';

/** Mirrors packer.TemplateWarning */
export function templateWarning(templ: string): string {
  const t = templ.trim();
  return t === '' || /\{\{-?\s*[^}]*\.(Number|Title|SiteTitle|Volume)\b/.test(t) ? '' : TEMPLATE_WARNING;
}

/** Total chapters in the One Piece feed (MangaDex, ch 598-1194) */
const CHAPTERS = OP_TITLES.ca.length; // 597
const PAGES = 12;
// a page's steps while downloading and while archiving, as core's
// DownloadWeight and ArchiveWeight: a bar fills 80% in the download
const DOWNLOAD_WEIGHT = 4;
const ARCHIVE_WEIGHT = 1;
const STEPS_PER_PAGE = DOWNLOAD_WEIGHT + ARCHIVE_WEIGHT;
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
  'check',
  'volumes',
  'volumes-running',
  'parts',
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
  /** The series lists no volumes */
  novolumes: boolean;
  /** The series splits its chapters in parts */
  parts: boolean;
  /** The fourth chapter fails halfway through download */
  fail: boolean;
  /** The sixth chapter comes out skipped (every page excluded by --skip-pages) */
  skip: boolean;
  /** Every 50th chapter is listed as locked */
  locked: boolean;
  /** Use production release versions and a macOS output path (for screenshots) */
  shot: boolean;
  /** Chapter rows start fetching their page list before they download */
  fetching: boolean;
  /** Start with EPUB as the output format */
  epub: boolean;
  /** With the update scenario: one-click update, its prompt open on load */
  prompt: boolean;
  /** The window's language preference the mock starts with */
  uiLanguage: LangPref;
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
  // multi's current language is the window's, see offersFor
  multi: { language: { values: ['ca', 'en', 'es'], all: true, current: 'en' }, scanlator: null },
  plus: { language: { values: ['en', 'es'], all: false, current: 'en' }, scanlator: null },
  groups: { language: null, scanlator: { values: ['Alpha', 'Delta'], all: true, current: '' } },
  none: { language: null, scanlator: null },
};

/** A site's offers; multi shows the window's language, which its list is in */
function offersFor(site: MockSite, lang: Lang): MockOffers {
  const offers = OFFERS[site];
  if (site !== 'multi' || offers.language === null) {
    return offers;
  }
  return { ...offers, language: { ...offers.language, current: lang } };
}

const en = createT('en');

/** A message as the Go side sends it: its key, its args, and the English text */
function msgOf(key: string, args?: Vars): Message {
  return args === undefined ? { key, text: en(key) } : { key, args, text: en(key, args) };
}

/** A rejection as the backend gives it: the message's JSON, which the window translates */
const reject = (key: string, args?: Vars): string => JSON.stringify(msgOf(key, args));

/** A job's phase: its status and its message, none when it has no phase */
const phaseOf = (m?: Message) => ({ status: m?.text ?? '', statusMsg: m });

/** A failed job's reason, with its message */
const failedWith = (m: Message) => ({ reason: m.text, reasonMsg: m });

/** The line for a filter that was reset, as the backend words it */
function fallbackLine(kind: 'language' | 'group', value: string, fallback: string): Message {
  const args = { value, fallback };
  if (kind === 'group') {
    if (fallback === 'all') return msgOf('log.groupFallbackAll', args);
    return fallback === '' ? msgOf('log.groupFallbackNone', args) : msgOf('log.groupFallbackValue', args);
  }
  if (fallback === 'all') return msgOf('log.languageFallbackAll', args);
  return fallback === '' ? msgOf('log.languageFallbackNone', args) : msgOf('log.languageFallbackValue', args);
}

const MOCK_ALERT = msgOf('browser.alertNamed', { browser: 'Brave', host: 'check.example.com' });

const REPO = 'https://github.com/elboletaire/manga-downloader';
const RELEASE_VERSION_DEV = 'v1.12.0';
const RELEASE_VERSION_SHOT = 'v1.19.0';
const UPDATE_VERSION_DEV = 'v1.13.0';
const UPDATE_VERSION_SHOT = 'v1.20.0';

function siteFrom(search: string): MockSite {
  const value = new URLSearchParams(search).get('site');
  return (SITES as readonly string[]).includes(value ?? '') ? (value as MockSite) : 'multi';
}

function scenarioFrom(search: string): Scenario {
  const value = new URLSearchParams(search).get('scenario');
  return (SCENARIOS as readonly string[]).includes(value ?? '') ? (value as Scenario) : 'empty';
}

/** ?lang=system is the preference that follows the system; anything else but en, es or ca is English */
function uiLanguageFrom(search: string): LangPref {
  const value = new URLSearchParams(search).get('lang');
  if (value === 'system') {
    return '';
  }
  return (LANGS as readonly string[]).includes(value ?? '') ? (value as LangPref) : 'en';
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
    novolumes: params.has('novolumes'),
    parts: params.has('parts'),
    fail: params.has('fail'),
    skip: params.has('skip'),
    locked: params.has('locked'),
    shot: params.has('shot'),
    fetching: params.has('fetching'),
    epub: params.has('epub'),
    prompt: params.has('prompt'),
    uiLanguage: uiLanguageFrom(search),
  };
}

// the extension each format's output carries; raw writes a folder without one
const EXTENSIONS: Record<Options['format'], string> = { cbz: '.cbz', cbr: '.cbr', epub: '.epub', raw: '' };

function defaultOptions(shot: boolean, epub: boolean): Options {
  return {
    outputDir: shot ? '/Users/reader/Downloads/Manga' : '/home/reader/Downloads',
    format: epub ? 'epub' : 'cbz',
    readingDirection: 'rtl',
    packAs: 'chapters',
    language: '',
    scanlator: '',
    convertImages: 'avif',
    skipPages: '',
    filenameTemplate: TEMPLATE_DEFAULT,
    browserVisible: false,
  };
}

/**
 * The newest `count` chapters, titled in `lang`. With withLanguages they are tagged with it, and every
 * fifth also appears once in `other` (the two lists share their chapters, in order). Volume 0: not collected.
 */
function mockChapters(
  count: number,
  lang: Lang,
  withLanguages: boolean,
  other: Lang,
  withVolumes = true,
  withLocked = false,
): Chapter[] {
  const list: Chapter[] = [];
  const rows = OP_TITLES[lang];
  const otherRows = OP_TITLES[other];
  for (let i = Math.max(0, rows.length - count); i < rows.length; i++) {
    const [number, title, vol] = rows[i];
    const volume = withVolumes ? vol : 0;
    const locked = withLocked && isMockLocked(number);
    list.push({ index: list.length, number, title, language: withLanguages ? lang : '', group: '', volume, locked });
    if (withLanguages && number % 5 === 0) {
      list.push({ index: list.length, number, title: otherRows[i][1], language: other, group: '', volume, locked });
    }
  }
  return list;
}

/**
 * A series published in parts: chapter 1 whole, then each chapter in two,
 * everything past chapter 4 locked but the second half of 19. The parts share
 * their chapter's One Piece title, plus their part, as the grabbers give them.
 */
function partsChapters(lang: Lang): Chapter[] {
  const list: Chapter[] = [];
  const withPart = (title: string, part: string) => (title === '' ? part : `${title} ${part}`);
  const add = (number: number, title: string, locked: boolean, part?: string) =>
    list.push({
      index: list.length,
      number,
      title,
      language: '',
      group: '',
      volume: 0,
      locked,
      ...(part ? { part, wholeTitle: titleOf(number) } : {}),
    });
  const newest = OP_TITLES[lang].slice(-20).map(([, title]) => title);
  const titleOf = (n: number) => newest[n - 1];
  add(1, titleOf(1), false);
  for (let n = 2; n <= 20; n++) {
    add(n, withPart(titleOf(n), 'Part 1'), n > 4, 'Part 1');
    add(n, withPart(titleOf(n), 'Part 2'), n > 4 && n !== 19, 'Part 2');
  }
  return list;
}

/** The error a locked chapter fails with, as core.ErrLocked says it */
const LOCKED_REASON = msgOf('job.reason.locked');

/** A chapter's job label: its title, or its number where the list has none */
const labelOf = (c: Chapter) => (c.title === '' ? `Chapter ${c.number}` : c.title);

/** Every 50th chapter is locked (a listing's coin lock), so a few show in dev. */
function isMockLocked(number: number): boolean {
  return number % 50 === 0;
}

/** Every chapter by Alpha, and every fifth again by Delta, so a number repeats in one language. */
function groupedChapters(list: Chapter[]): Chapter[] {
  const out: Chapter[] = [];
  for (const c of list) {
    out.push({ ...c, group: 'Alpha' });
    if (c.number % 5 === 0) {
      out.push({ ...c, group: 'Delta' });
    }
  }

  return out.map((c, index) => ({ ...c, index }));
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
    novolumes,
    parts: withParts,
    fail,
    skip,
    locked,
    shot,
    fetching,
    epub,
    prompt,
    uiLanguage,
  } = {
    scenario: 'empty' as Scenario,
    freeze: false,
    dupes: false,
    chapters: CHAPTERS,
    site: 'multi' as MockSite,
    novolumes: false,
    parts: false,
    fail: false,
    skip: false,
    locked: false,
    shot: false,
    fetching: false,
    epub: false,
    prompt: false,
    uiLanguage: 'en' as LangPref,
    ...(typeof setup === 'string' ? { scenario: setup } : setup),
  };
  // the window's language: the one a multi site shows and lists in
  const uiLang = resolveLanguage(uiLanguage, typeof navigator === 'undefined' ? [] : navigator.languages);
  const offers = offersFor(site, uiLang);
  const options = defaultOptions(shot, epub);
  // the app preferences, kept as the backend keeps them in gui.json
  const prefs: Prefs = { uiLanguage, checkUpdates: true };
  let url = '';
  let loading = false;
  let loadError: Message | undefined;
  let loaded = false;
  let title = '';
  let chapters: Chapter[] = [];
  let selected = new Set<number>();
  // the Volumes box and the site's volumes, as the backend keeps them per load
  let volumesText = '';
  let volumesSite = '';
  let volumesListed = 0;
  let applied = { language: '', scanlator: '' };
  // whether a load has been attempted, so the filters are the series' own choices
  let identified = false;
  let running = false;
  let cancelling = false;
  let runChapters = 0;
  // the bundle option as the run started with it, as the backend's RunInfo
  let runBundle = false;
  let runVolumes = false;
  let runParts = false;
  let runRows = 0;
  let jobs: Job[] = [];
  let summary: Summary | null = null;
  let log: LogLine[] = [];
  let timer: ReturnType<typeof setInterval> | undefined;
  // the loaded site needs the browser window, and the alert about it
  let browserAuto = false;
  let browserAlert: Message | undefined;

  const stateBus = new Bus<State>();
  const chaptersBus = new Bus<Chapter[]>();
  const selectionBus = new Bus<number[]>();
  const logBus = new Bus<LogLine[]>();

  const selectionOf = () => [...selected].sort((a, b) => a - b);
  const pushLog = (level: LogLine['level'], line: string | Message) => {
    const entry: LogLine = typeof line === 'string' ? { level, text: line } : { level, text: line.text, msg: line };
    log = [...log, entry].slice(-500);
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
  // the filter's language, else the site's current one (the window's where the site has no languages)
  const listedLanguage = (): Lang => {
    const offer = offers.language;
    if (offer === null) return uiLang;
    return (offer.values.includes(options.language) ? options.language : offer.current) as Lang;
  };
  // the language a list's repeated chapters are in: the site's first other one
  const otherLanguage = (lang: Lang): Lang => {
    const values: readonly string[] = offers.language?.values ?? LANGS;
    return values.find((v) => v !== lang) as Lang;
  };
  // a typed value the series doesn't offer falls back as the backend does: to
  // "all" where the site takes it, else to the site's current value, else to no
  // filter. A value that matches one ignoring case takes the listed spelling.
  const fallbackFor = (kind: 'language' | 'scanlator', line: 'language' | 'group'): void => {
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
    pushLog('info', fallbackLine(line, typed, fallback));
  };

  const releaseVersion = shot ? RELEASE_VERSION_SHOT : RELEASE_VERSION_DEV;
  // update stands for a release build, and so does every scenario with &shot,
  // which reports a release version: the status bar links it to its release
  const isReleaseBuild = (sc: Scenario) => sc === 'update';
  const reportsRelease = shot || isReleaseBuild(scenario);

  const snapshot = (): State => ({
    url,
    loading,
    loaded,
    loadError: loadError?.text ?? '',
    loadErrorMsg: loadError,
    title,
    chapterCount: chapters.length,
    hasDuplicates: chapters.some((c) => c.language !== ''),
    hasGroupDuplicates: chapters.some(
      (c) => c.group !== '' && chapters.some((d) => d !== c && d.number === c.number && d.language === c.language),
    ),
    stale: loaded && !loading && (options.language !== applied.language || options.scanlator !== applied.scanlator),
    loadedLanguage: applied.language,
    loadedScanlator: applied.scanlator,
    languages: filterOf('language'),
    scanlators: filterOf('scanlator'),
    selectedCount: selected.size,
    running,
    cancelling,
    run: { chapters: runChapters, bundle: runBundle, volumes: runVolumes, rows: runRows },
    jobs: jobs.map((j) => ({ ...j })),
    summary: summary ? { ...summary, failures: [...summary.failures] } : null,
    options: { ...options },
    volumes: { text: volumesText, site: volumesSite, listed: volumesListed },
    version: reportsRelease ? releaseVersion : 'v0.0.0-mock',
    releaseURL: reportsRelease ? `${REPO}/releases/tag/${releaseVersion}` : '',
    // off hides the update; on brings it back
    update:
      isReleaseBuild(scenario) && prefs.checkUpdates
        ? {
            latest: shot ? UPDATE_VERSION_SHOT : UPDATE_VERSION_DEV,
            url: `${REPO}/releases/latest`,
            oneClick: !shot || prompt,
            ...(prompt && { prompt: true }),
          }
        : null,
    browserVisibleAuto: browserAuto,
    browserAlert: browserAlert?.text ?? '',
    browserAlertMsg: browserAlert,
    filenameTemplateDefault: TEMPLATE_DEFAULT,
    filenameTemplateWarning:
      options.packAs === 'chapters' || options.packAs === 'parts' ? templateWarning(options.filenameTemplate) : '',
    // the wire omits uiLanguage when it follows the system, as gui.json does
    prefs: prefs.uiLanguage === '' ? { checkUpdates: prefs.checkUpdates } : { ...prefs },
  });
  const push = () => stateBus.emit(snapshot());

  function finishLoad(): void {
    loading = false;
    loaded = true;
    identified = true;
    fallbackFor('language', 'language');
    fallbackFor('scanlator', 'group');
    title = 'One Piece';
    const lang = listedLanguage();
    chapters =
      withParts || scenario === 'parts'
        ? partsChapters(lang)
        : mockChapters(
            chapterCount,
            lang,
            dupes || url.includes('dupes') || options.language === 'all',
            otherLanguage(lang),
            !novolumes,
            locked,
          );
    if (offers.scanlator !== null) {
      chapters = groupedChapters(chapters);
    }
    volumesSite = siteVolumes(chapters);
    volumesListed = listedVolumes(chapters);
    volumesText = '';
    applied = { language: options.language, scanlator: options.scanlator };
    selected = new Set<number>();
    jobs = [];
    summary = null;
    pushLog('info', msgOf('log.loaded', { title, count: chapters.length }));
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
    loadError = msgOf('errors.loadChapters', { reason: 'unexpected status 403 Forbidden' });
    pushLog('error', loadError);
    chaptersBus.emit([]);
    selectionBus.emit([]);
    push();
  }

  function finishRun(): void {
    clearInterval(timer);
    timer = undefined;
    running = false;
    // a volume's chapters count as saved with it, as the backend counts them
    const saved = jobs
      .filter((j) => j.state === 'done')
      .reduce((n, j) => n + (j.volume ? (volumeChapters.get(j.title) ?? 0) : 1), 0);
    const volumes = jobs.filter((j) => j.volume && j.state === 'done').length;
    const failures = jobs
      .filter((j) => j.state === 'failed')
      .map((j) => ({ label: j.title, reason: j.reason, reasonMsg: j.reasonMsg }));
    const skipped = jobs.filter((j) => j.state === 'skipped').length;
    const bundled = runBundle;
    summary = {
      dir: options.outputDir,
      cancelled: cancelling,
      saved: bundled && failures.length === 0 && !cancelling ? runChapters : saved,
      volumes,
      parts: runParts,
      bundled,
      bundle: bundled && !cancelling ? `${title} 1-${runChapters}${EXTENSIONS[options.format]}` : '',
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
  // how many chapters each volume row of the run holds, by title
  let volumeChapters = new Map<string, number>();
  // with &fetching, the ticks a chapter's row has left fetching its page list
  const fetchLeft = new Map<string, number>();
  const FETCH_TICKS = 4;

  /** A tick's steps: the pace it had when a page was two steps, scaled to STEPS_PER_PAGE. */
  function ticks(n: number): number {
    return Math.round((n * STEPS_PER_PAGE) / 2);
  }

  /** The step at which a bar of total steps switches from downloading to archiving. */
  function downloadEnd(total: number): number {
    return (total * DOWNLOAD_WEIGHT) / STEPS_PER_PAGE;
  }

  /** One step of the simulated run. Returns false once it's over. */
  function step(): boolean {
    if (runBundle) {
      const j = jobs[0];
      const done = Math.min(j.total, j.done + ticks(4));
      const fetched = done < downloadEnd(j.total);
      jobs = [
        {
          ...j,
          done,
          state: done >= j.total ? 'done' : fetched ? 'downloading' : 'archiving',
          ...phaseOf(
            done >= j.total
              ? undefined
              : fetched
                ? msgOf('job.phase.downloadingLabel', {
                    label: `Chapter ${Math.floor((done / downloadEnd(j.total)) * runChapters) + 1}`,
                  })
                : msgOf('job.phase.creatingBundle', { range: `1-${runChapters}` }),
          ),
        },
      ];
      return done < j.total;
    }

    // the volumes all have rows from the start; two move at a time, as the
    // backend downloads chapters in order
    let movingVolumes = 0;
    jobs = jobs.map((j) => {
      if (!j.volume || (j.state !== 'downloading' && j.state !== 'archiving') || movingVolumes >= 2) {
        return j;
      }
      movingVolumes++;
      const done = Math.min(j.total, j.done + ticks(5));
      if (done >= j.total) {
        return { ...j, done, state: 'done', ...phaseOf() };
      }
      const packing = done >= downloadEnd(j.total);
      return {
        ...j,
        done,
        state: packing ? 'archiving' : 'downloading',
        ...phaseOf(packing ? msgOf('job.phase.packing') : undefined),
      };
    });

    jobs = jobs.map((j) => {
      const left = fetchLeft.get(j.title);
      if (j.state !== 'fetching' || left === undefined) {
        return j;
      }
      if (left > 1) {
        fetchLeft.set(j.title, left - 1);
        return j;
      }
      fetchLeft.delete(j.title);
      return { ...j, state: 'downloading', total: PAGES * STEPS_PER_PAGE, ...phaseOf() };
    });

    jobs = jobs.map((j, idx) => {
      if (j.volume || (j.state !== 'downloading' && j.state !== 'archiving')) {
        return j;
      }
      if (fail && idx === 3 && j.done >= downloadEnd(j.total) / 2) {
        return { ...j, state: 'failed', ...failedWith(msgOf('job.reason.download', { reason: 'page 5 returned 404 Not Found' })) };
      }
      // uneven speeds, so the rows don't move in lockstep
      const done = Math.min(j.total, j.done + ticks(1 + ((j.number * 7) % 4)));
      const state = done >= j.total ? 'done' : done >= downloadEnd(j.total) ? 'archiving' : 'downloading';
      return { ...j, done, state };
    });

    const active = jobs.filter(
      (j) => !j.volume && (j.state === 'fetching' || j.state === 'downloading' || j.state === 'archiving'),
    ).length;
    if (!cancelling && active < 3 && pending.length > 0) {
      const c = pending.shift()!;
      if (c.locked) {
        // as the backend's FetchFailed: a listed lock fails before any fetch,
        // with a row unless the run is a bundle
        if (!runBundle) {
          jobs = [
            ...jobs,
            {
              title: labelOf(c),
              tags: '',
              number: c.number,
              total: 0,
              done: 0,
              state: 'failed',
              reason: LOCKED_REASON.text,
              reasonMsg: LOCKED_REASON,
              status: '',
              volume: false,
            },
          ];
        }
        pushLog('error', msgOf('log.errorFetching', { label: labelOf(c), reason: LOCKED_REASON.text }));
        return true;
      }
      if (skip && jobs.filter((j) => !j.volume).length === 5) {
        // as the backend's Skipped event: a row with nothing to download
        jobs = [
          ...jobs,
          {
            title: labelOf(c),
            tags: '',
            number: c.number,
            total: 0,
            done: 0,
            state: 'skipped',
            reason: 'every page excluded by --skip-pages',
            reasonMsg: msgOf('job.reason.skipped'),
            status: '',
            volume: false,
          },
        ];
        pushLog('warn', msgOf('log.skipping', { label: labelOf(c) }));
        return true;
      }
      if (fetching) {
        fetchLeft.set(labelOf(c), FETCH_TICKS);
      }
      jobs = [
        ...jobs,
        {
          title: labelOf(c),
          tags: '',
          number: c.number,
          total: fetching ? 0 : PAGES * STEPS_PER_PAGE,
          done: 0,
          state: fetching ? 'fetching' : 'downloading',
          reason: '',
          ...phaseOf(fetching ? msgOf('job.status.fetching') : undefined),
          volume: false,
        },
      ];
    }

    return (
      jobs.some((j) => j.state === 'fetching' || j.state === 'downloading' || j.state === 'archiving') ||
      (!cancelling && pending.length > 0)
    );
  }

  function beginRun(list: Chapter[]): void {
    running = true;
    cancelling = false;
    runChapters = list.length;
    runBundle = options.packAs === 'bundle';
    runVolumes = options.packAs === 'volumes' || options.packAs === 'parts';
    runParts = options.packAs === 'parts';
    summary = null;
    pending = [...list];
    volumeChapters = new Map();
    fetchLeft.clear();
    runRows = runBundle ? 1 : list.length;
    if (runParts) {
      // a chapter's parts make one row, as a volume's chapters do
      const groups = groupParts(list);
      pending = groups.filter((g) => g.chapters.length === 1).map((g) => g.chapters[0]);
      runRows = groups.length;
      jobs = groups
        .filter((g) => g.chapters.length > 1)
        .map((g) => {
          const titleMsg = msgOf('job.title.volumeParts', { volume: `Chapter ${g.number}`, count: g.chapters.length });
          const title = titleMsg.text;
          volumeChapters.set(title, g.chapters.length);
          return {
            title,
            titleMsg,
            tags: '',
            number: g.number,
            total: g.chapters.length * PAGES * STEPS_PER_PAGE,
            done: 0,
            state: 'downloading' as const,
            reason: '',
            ...phaseOf(msgOf('job.phase.waiting')),
            volume: true,
          };
        });
      return;
    }
    if (runVolumes) {
      // the backend makes a row per volume up front, and runs the chapters
      // outside them one by one
      const grouping = groupChapters(list, parseVolumes(volumesText));
      pending = grouping.loose;
      runRows = grouping.volumes.length + grouping.loose.length;
      jobs = grouping.volumes.map((v) => {
        const first = v.chapters[0].number;
        const last = v.chapters[v.chapters.length - 1].number;
        const vol = `Vol. ${String(v.number).padStart(2, '0')}`;
        const titleMsg =
          first === last
            ? msgOf('job.title.volumeChapter', { volume: vol, first })
            : msgOf('job.title.volume', { volume: vol, first, last });
        const title = titleMsg.text;
        volumeChapters.set(title, v.chapters.length);
        return {
          title,
          titleMsg,
          tags: '',
          number: first,
          total: v.chapters.length * PAGES * STEPS_PER_PAGE,
          done: 0,
          state: 'downloading' as const,
          reason: '',
          ...phaseOf(msgOf('job.phase.waiting')),
          volume: true,
        };
      });
      return;
    }
    const bundleTitle = msgOf('job.title.bundle', { range: `1-${list.length}` });
    jobs = runBundle
      ? [
          {
            title: bundleTitle.text,
            titleMsg: bundleTitle,
            tags: '',
            number: 0,
            total: list.length * PAGES * STEPS_PER_PAGE,
            done: 0,
            state: 'downloading',
            reason: '',
            ...phaseOf(msgOf('job.phase.gathering')),
            volume: false,
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

  if (scenario === 'check') {
    url = 'https://check.example.com/manga/one-piece';
    loading = true;
    browserAuto = true;
    browserAlert = MOCK_ALERT;
    if (!freeze) {
      setTimeout(() => {
        browserAlert = undefined;
        finishLoad();
      }, 4000);
    }
  } else if (scenario === 'loading') {
    url = 'https://example.com/manga/one-piece';
    loading = true;
    if (!freeze) {
      setTimeout(finishLoad, 2500);
    }
  } else if (scenario === 'error') {
    url = 'https://unknown.example.org/manga/some-series';
    failLoad();
    log = [];
    pushLog('info', `Loading ${url}`);
    if (loadError !== undefined) pushLog('error', loadError);
  } else if (scenario !== 'empty') {
    url = 'https://example.com/manga/one-piece';
    finishLoad();
    for (const c of [...chapters.slice(0, 12), ...chapters.slice(40, 52)]) {
      if (!c.locked) {
        selected.add(c.index);
      }
    }
    selectionBus.emit(selectionOf());
  }
  if (scenario === 'stale') {
    options.language = 'es';
  }
  if (scenario === 'update') {
    // A realistic site-check failure: selector timeout including the full series URL.
    // The URL is long enough to overflow the status bar without the CSS fix.
    if (!shot) {
      pushLog(
        'warn',
        msgOf('log.siteCheck', {
          reason: 'timed out waiting for "li.list-group-item, img.PB0mN" at https://example.com/manga/the-cartographer-of-lost-harbours-collected-edition',
        }),
      );
    }
  }
  if (scenario === 'volumes' || scenario === 'volumes-running') {
    options.packAs = 'volumes';
    // three volumes and a half: the first 32 chapters (598-629) span volumes 61-64
    selected = new Set(
      chapters
        .slice(0, Math.min(32, chapters.length))
        .filter((c) => !c.locked)
        .map((c) => c.index),
    );
    selectionBus.emit(selectionOf());
  }
  if (scenario === 'parts') {
    url = 'https://example.com/manga/one-piece';
    options.packAs = 'parts';
    // every part that isn't locked: chapters 1-4, and the one open part of 19
    selected = new Set(chapters.filter((c) => !c.locked).map((c) => c.index));
    selectionBus.emit(selectionOf());
  }
  if (scenario === 'volumes-running') {
    const picked = chapters.filter((c) => selected.has(c.index));
    pushLog('info', msgOf('log.downloading', { count: picked.length, title }));
    beginRun(picked);
    for (let i = 0; i < 12 && step(); i++) {
      // advancing
    }
    if (!freeze) {
      timer = setInterval(tick, TICK_MS);
    }
  }
  if (scenario === 'running' || scenario === 'cancelling' || scenario === 'bundle' || scenario === 'done') {
    options.packAs = scenario === 'bundle' ? 'bundle' : 'chapters';
    const picked = chapters.filter((c) => selected.has(c.index)).slice(0, 12);
    pushLog('info', msgOf('log.downloading', { count: picked.length, title }));
    beginRun(picked);
    // run the simulation forward to an interesting point
    const steps = scenario === 'done' ? Infinity : scenario === 'bundle' ? 8 : 14;
    for (let i = 0; i < steps && step(); i++) {
      // advancing
    }
    if (scenario === 'cancelling') {
      cancelling = true;
      pushLog('info', msgOf('log.cancelling'));
    }
    if (scenario === 'done') {
      // a conversion warning on a chapter of the run (the second, 599 by default)
      const warned = picked[1] ?? picked[0];
      if (warned !== undefined) {
        pushLog('warn', `Chapter ${warned.number}: page 7 is AVIF but could not be converted; kept as is`);
      }
      finishRun();
    } else if (!freeze) {
      timer = setInterval(tick, TICK_MS);
    }
  }
  push();

  // a load of next's URL: the body of the load action, shared with setOptionsAndLoad
  async function startLoad(next: string): Promise<void> {
    const trimmed = next.trim();
    if (trimmed === '') {
      throw reject('errors.enterURL');
    }
    if (loading || running) {
      throw reject('errors.busy');
    }
    url = trimmed;
    loadError = undefined;
    loading = true;
    browserAuto = trimmed.includes('check');
    browserAlert = browserAuto ? MOCK_ALERT : undefined;
    push();
    await new Promise((resolve) => setTimeout(resolve, browserAuto ? 3000 : 400));
    browserAlert = undefined;
    if (trimmed.includes('unknown')) {
      failLoad();
      return;
    }
    finishLoad();
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
      await startLoad(next);
    },
    async setOptionsAndLoad(next, target) {
      if (loading || running) {
        throw reject('errors.busy');
      }
      Object.assign(options, next);
      await startLoad(target);
    },
    async clear() {
      if (loading || running) {
        throw reject('errors.busy');
      }
      url = '';
      loaded = false;
      loadError = undefined;
      identified = false;
      browserAuto = false;
      browserAlert = undefined;
      applied = { language: '', scanlator: '' };
      title = '';
      chapters = [];
      selected = new Set<number>();
      volumesText = '';
      volumesSite = '';
      volumesListed = 0;
      jobs = [];
      summary = null;
      chaptersBus.emit([]);
      selectionBus.emit([]);
      push();
    },
    async setSelected(index, on) {
      if (loading || running) {
        throw reject('errors.busy');
      }
      if (index < 0 || index >= chapters.length) {
        return;
      }
      if (on && !chapters[index].locked) {
        selected.add(index);
      } else if (!on) {
        selected.delete(index);
      }
      selectionBus.emit(selectionOf());
      push();
    },
    async setSelectedMany(indices, on) {
      if (loading || running) {
        throw reject('errors.busy');
      }
      if (indices.some((index) => index < 0 || index >= chapters.length)) {
        throw reject('errors.chapterMissing', { index: indices.find((index) => index < 0 || index >= chapters.length) as number });
      }
      for (const index of indices) {
        if (on) {
          if (!chapters[index].locked) {
            selected.add(index);
          }
        } else {
          selected.delete(index);
        }
      }
      selectionBus.emit(selectionOf());
      push();
    },
    async setAll(on) {
      if (loading || running) {
        throw reject('errors.busy');
      }
      selected = new Set(on ? chapters.filter((c) => !c.locked).map((c) => c.index) : []);
      selectionBus.emit(selectionOf());
      push();
    },
    async selectRange(text) {
      if (loading || running) {
        throw reject('errors.busy');
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
          if (c.number >= a && c.number <= hi && !c.locked) {
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
        throw reject('errors.busy');
      }
      Object.assign(options, next);
      push();
    },
    async setVolumes(text) {
      if (loading || running) {
        throw reject('errors.busy');
      }
      try {
        parseVolumes(text);
      } catch (e) {
        throw (e as Error).message;
      }
      volumesText = text.trim();
      push();
    },
    async chooseFolder() {
      if (loading || running) {
        throw reject('errors.busy');
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
    async readClipboard() {
      return 'https://example.com/manga/one-piece';
    },
    async start() {
      if (loading || running) {
        throw reject('errors.busy');
      }
      if (!loaded) {
        throw reject('errors.loadFirst');
      }
      if (options.language !== applied.language || options.scanlator !== applied.scanlator) {
        throw reject('errors.filtersChanged');
      }
      const picked = chapters.filter((c) => selected.has(c.index));
      if (picked.length === 0) {
        throw reject('errors.selectOne');
      }
      if (
        options.packAs === 'volumes' &&
        groupChapters(picked, parseVolumes(volumesText)).volumes.length === 0 &&
        volumesListed === 0 &&
        volumesText === ''
      ) {
        throw reject('errors.noVolumes');
      }
      pushLog('info', msgOf('log.downloading', { count: picked.length, title }));
      simulateRun(picked);
    },
    async update() {
      pushLog('info', msgOf('log.downloadingUpdate', { version: UPDATE_VERSION_DEV }));
      await new Promise((resolve) => setTimeout(resolve, 1500));
      const reason = 'the mock has no installer';
      pushLog('error', msgOf('errors.updateDownload', { reason }));
      throw reject('errors.updateDownload', { reason });
    },
    async showBrowser() {
      pushLog('info', 'would bring the browser window to the front (mock)');
    },
    async dismissBrowserAlert() {
      browserAlert = undefined;
      push();
    },
    // like the backend's, never refused while a load or run is going on
    async setUILanguage(pref) {
      prefs.uiLanguage = pref;
      push();
    },
    async setCheckUpdates(on) {
      prefs.checkUpdates = on;
      push();
    },
    async cancel() {
      if (!running || cancelling) {
        return;
      }
      cancelling = true;
      pushLog('info', msgOf('log.cancelling'));
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
