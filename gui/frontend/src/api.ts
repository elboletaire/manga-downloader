// The one module the UI talks to the backend through. createWailsApi wraps the
// bindings Wails generates into wailsjs/; createMockApi (mock.ts) stands in for
// them in a plain browser, so the UI can be worked on without the Go app.

import * as app from '../wailsjs/go/main/App';
import { BrowserOpenURL, EventsOn } from '../wailsjs/runtime/runtime';
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
  setSelected(index: number, on: boolean): Promise<void>;
  /** Selects or deselects every index at once; rejects, changing nothing, if one is out of range */
  setSelectedMany(indices: number[], on: boolean): Promise<void>;
  setAll(on: boolean): Promise<void>;
  /** Selects the chapters in the range text; resolves with how many matched */
  selectRange(text: string): Promise<number>;
  setOptions(options: Options): Promise<void>;
  /** Native folder picker; resolves with "" when cancelled */
  chooseFolder(): Promise<string>;
  openFolder(): Promise<void>;
  /** Opens the URL in the system's browser, never in the window */
  openURL(url: string): void;
  start(): Promise<void>;
  cancel(): Promise<void>;

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
    setSelected: (index, on) => app.SetSelected(index, on),
    setSelectedMany: (indices, on) => app.SetSelectedMany(indices, on),
    setAll: (on) => app.SetAll(on),
    selectRange: (text) => app.SelectRange(text),
    setOptions: (options) => app.SetOptions(options as unknown as Parameters<typeof app.SetOptions>[0]),
    chooseFolder: () => app.ChooseFolder(),
    openFolder: () => app.OpenFolder(),
    openURL: (url) => BrowserOpenURL(url),
    start: () => app.Start(),
    cancel: () => app.Cancel(),

    onState: (cb) => state.subscribe(cb),
    onChapters: (cb) => chapters.subscribe(cb),
    onSelection: (cb) => selection.subscribe(cb),
    onLog: (cb) => log.subscribe(cb),
  };
}
