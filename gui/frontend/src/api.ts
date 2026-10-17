// The one module the UI talks to the backend through. createWailsApi wraps the
// bindings Wails generates into wailsjs/; createMockApi (mock.ts) stands in for
// them in a plain browser, so the UI can be worked on without the Go app.

import * as app from '../wailsjs/go/main/App';
import { BrowserOpenURL, ClipboardGetText, EventsOn } from '../wailsjs/runtime/runtime';
import type { LangPref } from './i18n';
import type { Chapter, LogLine, Options, State } from './types';

export type Unsubscribe = () => void;

export interface Api {
  // Snapshots, for the first render. The events below keep them current.
  state(): Promise<State>;
  chapters(): Promise<Chapter[]>;
  selection(): Promise<number[]>;
  log(): Promise<LogLine[]>;

  // Actions. A rejected promise carries the reason as a string (or Error).
  load(url: string): Promise<void>;
  /** Forgets the loaded series (URL, chapters, selection, load error); refused while one loads or a download runs */
  clear(): Promise<void>;
  setSelected(index: number, on: boolean): Promise<void>;
  /** Selects or deselects every index at once; rejects, changing nothing, if one is out of range */
  setSelectedMany(indices: number[], on: boolean): Promise<void>;
  setAll(on: boolean): Promise<void>;
  /** Selects the chapters in the range text; resolves with how many matched */
  selectRange(text: string): Promise<number>;
  setOptions(options: Options): Promise<void>;
  /** setOptions and then load(url), in one step: the options a selector picked are never seen unapplied */
  setOptionsAndLoad(options: Options, url: string): Promise<void>;
  /** Sets the Volumes box ("1-7/8-16", "" for the site's own); rejects a value that doesn't parse */
  setVolumes(text: string): Promise<void>;
  /** Native folder picker; resolves with "" when cancelled */
  chooseFolder(): Promise<string>;
  openFolder(): Promise<void>;
  /** Opens the URL in the system's browser, never in the window */
  openURL(url: string): void;
  /** The clipboard's text, for the Paste button; rejects when the runtime isn't there */
  readClipboard(): Promise<string>;
  start(): Promise<void>;
  cancel(): Promise<void>;
  /** Downloads and runs the installer, then quits; rejects with the reason (also logged) when it can't */
  update(): Promise<void>;
  /** Brings the browser window to the front again */
  showBrowser(): Promise<void>;
  /** Hides browserAlert; the window and the load waiting for it go on */
  dismissBrowserAlert(): Promise<void>;
  /** Saves the window's language; '' follows the system. Takes effect at once in the window. */
  setUILanguage(pref: LangPref): Promise<void>;
  setCheckUpdates(on: boolean): Promise<void>;

  // Pushes. Each returns a function that stops the callback.
  onState(cb: (s: State) => void): Unsubscribe;
  onChapters(cb: (c: Chapter[]) => void): Unsubscribe;
  onSelection(cb: (indices: number[]) => void): Unsubscribe;
  onLog(cb: (lines: LogLine[]) => void): Unsubscribe;
}

/** Events are registered once each; callbacks are fanned out from a set */
function fanOut<T>(event: string): { subscribe(cb: (v: T) => void): Unsubscribe } {
  const callbacks = new Set<(v: T) => void>();
  let registered = false;

  return {
    subscribe(cb) {
      callbacks.add(cb);
      if (!registered) {
        registered = true;
        EventsOn(event, (v: T) => {
          for (const fn of callbacks) {
            fn(v);
          }
        });
      }

      return () => {
        callbacks.delete(cb);
      };
    },
  };
}

export function createWailsApi(): Api {
  const state = fanOut<State>('state');
  const chapters = fanOut<Chapter[]>('chapters');
  const selection = fanOut<number[]>('selection');
  const log = fanOut<LogLine[]>('log');

  return {
    // The bindings' types are structurally the same as ours; the cast only
    // narrows the string-typed enums the Go side fills in.
    state: () => app.State() as unknown as Promise<State>,
    chapters: () => app.Chapters() as unknown as Promise<Chapter[]>,
    selection: () => app.Selection(),
    log: () => app.Log() as unknown as Promise<LogLine[]>,

    load: (url) => app.Load(url),
    clear: () => app.Clear(),
    setSelected: (index, on) => app.SetSelected(index, on),
    setSelectedMany: (indices, on) => app.SetSelectedMany(indices, on),
    setAll: (on) => app.SetAll(on),
    selectRange: (text) => app.SelectRange(text),
    setOptions: (options) => app.SetOptions(options as unknown as Parameters<typeof app.SetOptions>[0]),
    setOptionsAndLoad: (options, url) =>
      app.SetOptionsAndLoad(options as unknown as Parameters<typeof app.SetOptionsAndLoad>[0], url),
    setVolumes: (text) => app.SetVolumes(text),
    chooseFolder: () => app.ChooseFolder(),
    openFolder: () => app.OpenFolder(),
    openURL: (url) => BrowserOpenURL(url),
    // async, so a missing runtime rejects rather than throwing into the click
    readClipboard: async () => ClipboardGetText(),
    start: () => app.Start(),
    cancel: () => app.Cancel(),
    update: () => app.Update(),
    showBrowser: () => app.ShowBrowser(),
    dismissBrowserAlert: () => app.DismissBrowserAlert(),
    setUILanguage: (pref) => app.SetUILanguage(pref),
    setCheckUpdates: (on) => app.SetCheckUpdates(on),

    onState: (cb) => state.subscribe(cb),
    onChapters: (cb) => chapters.subscribe(cb),
    onSelection: (cb) => selection.subscribe(cb),
    onLog: (cb) => log.subscribe(cb),
  };
}
