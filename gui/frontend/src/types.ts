// Types of the data the Go App sends to the window. They mirror the JSON of
// the DTOs in gui/app.go and gui/store.go; keep the two in step.

import type { LangPref } from './i18n';

export type JobState = 'fetching' | 'downloading' | 'archiving' | 'done' | 'failed' | 'skipped';

export type LogLevel = 'info' | 'warn' | 'error';

/** How the chapters are packed: one file each, one per chapter with its parts merged, one per volume, or one for everything */
export type PackAs = 'chapters' | 'parts' | 'volumes' | 'bundle';

export interface Options {
  outputDir: string;
  format: 'cbz' | 'cbr' | 'raw' | 'epub';
  /** The way an EPUB's pages turn: manga reads right to left. Only EPUB output reads it */
  readingDirection: 'rtl' | 'ltr';
  packAs: PackAs;
  language: string;
  scanlator: string;
  /** Comma-separated: "avif", "avif,webp", "webp" or "none". See convertImages.ts. */
  convertImages: string;
  /** The quality (1-100) pages re-encoded to JPEG are written at, by convertImages and compress */
  jpegQuality: number;
  /** Re-encodes PNG pages to JPEG when that's smaller */
  compress: boolean;
  skipPages: string;
  /** Leaves out the chapters, volumes and bundles whose file is already in the folder */
  skipExisting: boolean;
  filenameTemplate: string;
  browserVisible: boolean;
}

export interface Run {
  chapters: number;
  bundle: boolean;
  /** The run packs one file per volume */
  volumes: boolean;
  /** How many progress rows the run ends up with: chapters, volumes plus the chapters outside them, or 1 bundle */
  rows: number;
}

/** A text the Go side sent with its key and args: translated by say() (see i18n), its text in English */
export interface Message {
  key: string;
  args?: Record<string, unknown>;
  text: string;
}

export interface Job {
  title: string;
  /** The title as a message, when the GUI wrote it (a bundle's or a volume's) */
  titleMsg?: Message;
  /** The release's language and group (" [en] [Alpha]"), shown apart from the title; empty when none */
  tags: string;
  number: number;
  /** Each page's download and archive steps are counted (core's DownloadWeight and ArchiveWeight, 4 + 1); a volume's row is a fixed scale instead */
  total: number;
  done: number;
  state: JobState;
  reason: string;
  reasonMsg?: Message;
  /** The bundle's or volume's current phase; empty for single chapters */
  status: string;
  statusMsg?: Message;
  /** The row of a volume rather than a chapter */
  volume: boolean;
}

/** A line of the run's failures: what failed, and why */
export interface Failure {
  /** The release or volume the line is about; "" when it names none */
  label: string;
  /** The reason in English, shown when the window has no message for it */
  reason: string;
  /** The reason as the window words it, when it has window words of its own */
  reasonMsg?: Message;
}

export interface Summary {
  dir: string;
  cancelled: boolean;
  saved: number;
  /** Volume files written */
  volumes: number;
  /** Those files are chapters with their parts merged */
  parts: boolean;
  bundled: boolean;
  bundle: string;
  skipped: number;
  /** Chapters left out because their file was already in the folder */
  existing: number;
  failures: Failure[];
  /** Set when a bundle couldn't be written */
  error: string;
  /** error as a message, when the window translates it */
  errorMsg?: Message;
}

/**
 * What a language or scanlator selector offers, for a series the site lists
 * them for. Values are what --language and --scanlator take.
 */
export interface FilterChoice {
  values: string[];
  /** The site also takes "all": every value, mixed */
  all: boolean;
  /** The value the loaded list shows; "" when the site shows no single one */
  selected: string;
}

/** The window's state, without the chapter list, selection and log */
export interface State {
  url: string;
  loading: boolean;
  loaded: boolean;
  loadError: string;
  /** loadError as a message, when the GUI wrote it */
  loadErrorMsg?: Message;
  title: string;
  chapterCount: number;
  hasDuplicates: boolean;
  /** One number is listed more than once in a language, and the groups tell the copies apart */
  hasGroupDuplicates: boolean;
  /** The language or scanlator changed since the list was fetched */
  stale: boolean;
  /** The language the list was fetched with (what the filter applies to) */
  loadedLanguage: string;
  /** The scanlator the list was fetched with */
  loadedScanlator: string;
  selectedCount: number;
  running: boolean;
  cancelling: boolean;
  run: Run;
  jobs: Job[];
  summary: Summary | null;
  options: Options;
  volumes: Volumes;
  /** null when the site doesn't list languages (or nothing is identified yet) */
  languages: FilterChoice | null;
  /** null when the site doesn't list scanlation groups */
  scanlators: FilterChoice | null;
  version: string;
  /** The GitHub release of this build; "" for a dev build, which has none */
  releaseURL: string;
  /** null when there's no newer release (or the check hasn't found one) */
  update: Update | null;
  /** The loaded site opens the browser window by itself, whatever options.browserVisible says */
  browserVisibleAuto: boolean;
  /** What to tell the user while the browser window waits for them; "" when it isn't */
  browserAlert: string;
  /** browserAlert as a message */
  browserAlertMsg?: Message;
  /** The filename template a blank option stands for */
  filenameTemplateDefault: string;
  /** What is wrong with options.filenameTemplate; "" when nothing */
  filenameTemplateWarning: string;
  prefs: Prefs;
}

/** App-only preferences: what the CLI has no flag for, so they aren't Options */
export interface Prefs {
  /** The window's language; '' follows the system. Go omits it when empty, so it can be absent */
  uiLanguage?: LangPref;
  /** Check GitHub for a newer release at startup */
  checkUpdates: boolean;
  /** Open on the simple view (logo and URL box) until a load starts */
  simpleStart: boolean;
}

/** The Volumes box, used when the options pack as volumes */
export interface Volumes {
  /** The volumes a run packs, as chapter ranges per volume ("1-7/8-16"); "" for the site's own */
  text: string;
  /** The site's volumes in the same syntax; "" when the site doesn't list them or chapter ranges can't group them as it does */
  site: string;
  /** How many volumes the site files chapters under, 0 when none does */
  listed: number;
}

/** A release newer than the running build, from the update check */
export interface Update {
  /** The newest release's tag, as GitHub names it */
  latest: string;
  /** The page the update is linked to: the latest release */
  url: string;
  /** Update() can install it: an installed copy, and its package and checksums on the release */
  oneClick: boolean;
  /** Open the update prompt on load; only the mock sets it (the 12-update-prompt screenshot) */
  prompt?: boolean;
}

export interface Chapter {
  index: number;
  number: number;
  title: string;
  /** Only set when hasDuplicates: the copies of one number differ by language */
  language: string;
  /** Only set when hasGroupDuplicates: the scanlation group that made the release */
  group: string;
  /** The volume the site files it under; 0 when the site doesn't say */
  volume: number;
  /** The listing marks it locked: it can't be selected, so no run takes it */
  locked: boolean;
  /** The part as its label writes it ("Part 2", "前編"); unset without one */
  part?: string;
  /** With a part: the title without it, '' when the site gives none */
  wholeTitle?: string;
  /** The site lists it without a number: a parts run packs it alone */
  unnumbered?: boolean;
}

export interface LogLine {
  level: LogLevel;
  /** The English text: what copying the log gives, and what shows when the window can't translate msg */
  text: string;
  /** The line as a translatable message, when the Go side sent one (see say() in i18n) */
  msg?: Message;
}
