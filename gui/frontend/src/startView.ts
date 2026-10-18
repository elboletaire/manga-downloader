// Which view the window shows: the simple one (the logo over the URL box) on a
// fresh start when the simpleStart pref is on, and the full one from the first
// load on. The decisions are pure so they can be tested without a window.

import type { State } from './types';

export type View = 'simple' | 'full';

type ViewState = Pick<State, 'prefs' | 'url' | 'loading' | 'loaded' | 'running' | 'summary'>;

/** Whether the window holds anything beyond an empty URL box: a series, a load, a run or its outcome */
function hasWork(s: ViewState): boolean {
  return s.loading || s.loaded || s.running || s.summary !== null || s.url !== '';
}

/** The view the window opens on */
export function bootView(s: ViewState): View {
  return s.prefs.simpleStart && !hasWork(s) ? 'simple' : 'full';
}

/** The view after a State: the simple one gives way once a load starts, and never comes back */
export function nextView(view: View, s: ViewState): View {
  return view === 'simple' && hasWork(s) ? 'full' : view;
}
