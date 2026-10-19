// Which view the window shows: the simple one (the logo over the URL box) while
// nothing is loaded and the simpleStart pref is on, else the full one. The
// decisions are pure so they can be tested without a window.

import type { State } from './types';

export type View = 'simple' | 'full';

type ViewState = Pick<State, 'prefs' | 'url' | 'loading' | 'loaded' | 'running' | 'summary'>;

/** Whether the window holds anything beyond an empty URL box: a series, a load, a run or its outcome */
export function hasWork(s: ViewState): boolean {
  return s.loading || s.loaded || s.running || s.summary !== null || s.url !== '';
}

/** The view the window opens on */
export function bootView(s: ViewState): View {
  return s.prefs.simpleStart && !hasWork(s) ? 'simple' : 'full';
}

/** The view after a State: once a load has started (`started`, sticky) it stays full, else it follows the pref */
export function nextView(view: View, s: ViewState, started: boolean): View {
  if (hasWork(s)) return 'full';
  if (started) return view;
  return s.prefs.simpleStart ? 'simple' : 'full';
}
