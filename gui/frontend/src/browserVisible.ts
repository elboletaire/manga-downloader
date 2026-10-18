// The "Show the browser window" switch. The option is the user's global
// choice; a site that needs the window (declared, or one that needed it
// earlier this session) gets it anyway, so the switch then shows on, locked,
// with a hint, without touching the saved option.

import type { T } from './i18n';
import type { State } from './types';

export interface BrowserSwitch {
  checked: boolean;
  disabled: boolean;
  hint: string;
}

export function browserSwitchOf(state: State, t: T): BrowserSwitch {
  const locked = state.loading || state.running;
  if (state.browserVisibleAuto) {
    return { checked: true, disabled: true, hint: t('options.browser.auto') };
  }
  return { checked: state.options.browserVisible, disabled: locked, hint: t('options.browser.hint') };
}
