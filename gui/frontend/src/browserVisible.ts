// The "Show the browser window" switch. The option is the user's global
// choice; a site that needs the window (declared, or one that needed it
// earlier this session) gets it anyway, so the switch then shows on, locked,
// with a hint, without touching the saved option.

import type { State } from './types';

export interface BrowserSwitch {
  checked: boolean;
  disabled: boolean;
  hint: string;
}

export const BROWSER_HINT = 'Rarely needed: the window opens by itself when a site asks for it.';
export const BROWSER_AUTO_HINT = 'Needed for this site: the window opens by itself.';

export function browserSwitchOf(state: State): BrowserSwitch {
  const locked = state.loading || state.running;
  if (state.browserVisibleAuto) {
    return { checked: true, disabled: true, hint: BROWSER_AUTO_HINT };
  }
  return { checked: state.options.browserVisible, disabled: locked, hint: BROWSER_HINT };
}
