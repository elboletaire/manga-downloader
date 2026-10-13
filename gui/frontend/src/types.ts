// Types of the data the Go App sends to the window. They mirror the JSON of
// the DTOs in gui/app.go and gui/store.go; keep the two in step.

export type JobState = 'downloading' | 'archiving' | 'done' | 'failed' | 'skipped';

export type LogLevel = 'info' | 'warn' | 'error';

/** How the chapters are packed: one file each, one per volume, or one for everything */
export type PackAs = 'chapters' | 'volumes' | 'bundle';

export interface Options {
  outputDir: string;
  format: 'cbz' | 'raw';
  packAs: PackAs;
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
  /** The run packs one file per volume */
  volumes: boolean;
  /** How many progress rows the run ends up with: chapters, volumes plus the chapters outside them, or 1 bundle */
  rows: number;
}

export interface Job {
  title: string;
  number: number;
  /** Each page's download and archive steps are counted (core's DownloadWeight and ArchiveWeight, 4 + 1); a volume's row is a fixed scale instead */
  total: number;
  done: number;
  state: JobState;
  reason: string;
  /** The bundle's or volume's current phase; empty for single chapters */
  status: string;
  /** The row of a volume rather than a chapter */
  volume: boolean;
}

export interface Summary {
  dir: string;
  cancelled: boolean;
  saved: number;
  /** Volume files written */
  volumes: number;
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
}

/** The Volumes box, used when the options pack as volumes */
export interface Volumes {
  /** The volumes a run packs, as chapter ranges per volume ("1-7/8-16"); "" for the site's own */
  text: string;
  /** The site's volumes in the same syntax; "" when the site doesn't say */
  site: string;
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
  /** The volume the site files it under; 0 when the site doesn't say */
  volume: number;
}

export interface LogLine {
  level: LogLevel;
  text: string;
}
