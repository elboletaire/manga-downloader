// Which face the mascot wears. Soft is the default. Happy is for a run that
// saved everything, and for the empty state; a failure never gets it (brand.md).

import type { State, Summary } from './types';

export type Expression = 'soft' | 'happy';

/**
 * A finished run that saved everything it was asked for. A skipped chapter
 * doesn't count against it: --skip-pages excluded all its pages on purpose.
 */
export function savedEverything(s: Summary): boolean {
  if (s.cancelled || s.error !== '' || s.failures.length > 0) {
    return false;
  }

  return s.bundled ? s.bundle !== '' : s.saved > 0;
}

/**
 * Whether a window that opens on this state celebrates: no load or run is
 * going, and the last run saved everything. The summary outlives the window,
 * so a reopened window still shows the last result's face.
 */
export function celebratesOnOpen(state: Pick<State, 'loading' | 'running' | 'summary'>): boolean {
  return !state.loading && !state.running && state.summary !== null && savedEverything(state.summary);
}

/**
 * The next value of the celebration after a state change. It ends as soon as
 * a load or a run starts. It starts only when a run ends (wasRunning) with
 * everything saved: a load that fails leaves the old summary behind, and must
 * not bring the face back.
 */
export function celebrating(
  was: boolean,
  wasRunning: boolean,
  state: Pick<State, 'loading' | 'running' | 'summary'>,
): boolean {
  if (state.loading || state.running) {
    return false;
  }
  if (wasRunning) {
    return state.summary !== null && savedEverything(state.summary);
  }

  return was;
}
