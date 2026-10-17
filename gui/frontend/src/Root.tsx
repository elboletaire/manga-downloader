import { useCallback, useEffect, useState } from 'react';
import type { Api, Unsubscribe } from './api';
import { App } from './App';
import type { LangPref } from './i18n';
import { I18nProvider } from './i18n/react';
import type { State } from './types';

/** The language a State asks for; gui.json omits uiLanguage when it follows the system */
export const prefOf = (state: State): LangPref => state.prefs.uiLanguage ?? '';

export interface PrefHandlers {
  /** The saved language: from the first snapshot, and from every push after it */
  onSaved(pref: LangPref): void;
  /** The first snapshot, or null when it can't be read (App then asks again and shows the failure) */
  onInitial(state: State | null): void;
}

/** Subscribes to the pushes before asking for the first snapshot, so nothing is missed */
export function watchPrefs(api: Api, handlers: PrefHandlers): Unsubscribe {
  let alive = true;
  const off = api.onState((state) => {
    if (alive) handlers.onSaved(prefOf(state));
  });
  api.state().then(
    (state) => {
      if (alive) {
        handlers.onSaved(prefOf(state));
        handlers.onInitial(state);
      }
    },
    () => {
      if (alive) handlers.onInitial(null);
    },
  );
  return () => {
    alive = false;
    off();
  };
}

/**
 * Gives the window its language before anything renders: nothing shows until the
 * first snapshot is read, so the saved language never flashes past the system's.
 */
export function Root({ api }: { api: Api }) {
  // undefined until the first snapshot is read; null when it can't be read
  const [initial, setInitial] = useState<State | null | undefined>(undefined);
  const [pref, setPref] = useState<LangPref>('');
  useEffect(() => watchPrefs(api, { onSaved: setPref, onInitial: setInitial }), [api]);
  const persist = useCallback((next: LangPref) => api.setUILanguage(next), [api]);

  if (initial === undefined) return null;
  return (
    <I18nProvider pref={pref} persist={persist}>
      <App api={api} initial={initial ?? undefined} />
    </I18nProvider>
  );
}
