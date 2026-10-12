// Types of the data the Go App sends to the window. They mirror the JSON of
// the DTOs in gui/app.go and gui/store.go; keep the two in step.

export type JobState = 'downloading' | 'archiving' | 'done' | 'failed' | 'skipped';

export type LogLevel = 'info' | 'warn' | 'error';

export interface Options {
  outputDir: string;
  format: 'cbz' | 'raw';
  bundle: boolean;
  language: string;
  scanlator: string;
  /** Comma-separated: "avif", "avif,webp", "webp" or "none". See convertImages.ts. */
  convertImages: string;
  skipPages: string;
  filenameTemplate: string;
  browserVisible: boolean;
}

export interface Run {
  chapters: number;
  bundle: boolean;
}

export interface Job {
  title: string;
  number: number;
  /** Pages counted twice: once downloaded, once archived */
  total: number;
  done: number;
  state: JobState;
  reason: string;
  /** The bundle's current phase; empty for single chapters */
  status: string;
}

export interface Summary {
  dir: string;
  cancelled: boolean;
  saved: number;
  bundled: boolean;
  bundle: string;
  skipped: number;
  failures: string[];
  /** Set when a bundle couldn't be written */
  error: string;
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
  title: string;
  chapterCount: number;
  hasDuplicates: boolean;
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
}

/** A release newer than the running build, from the update check */
export interface Update {
  /** The newest release's tag, as GitHub names it */
  latest: string;
  /** The page the update is linked to: the latest release */
  url: string;
}

export interface Chapter {
  index: number;
  number: number;
  title: string;
  /** Only set when hasDuplicates: the copies of one number differ by language */
  language: string;
}

export interface LogLine {
  level: LogLevel;
  text: string;
}
