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
  version: string;
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
