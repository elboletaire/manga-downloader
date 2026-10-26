import { useCallback, useEffect, useState } from 'react';
import type { Api, Unsubscribe } from './api';
import { App } from './App';
import type { LangPref } from './i18n';
import { I18nProvider } from './i18n/react';
import type { State } from './types';

/** The system's raw locale as the browser reports it: its first language, else its language */
export const systemLocale = (): string => navigator.languages?.[0] || navigator.language || '';

/** Tells the backend the system's locale once; its update check waits for this report */
export function reportSystemLocale(api: Api): void {
  void api.setSystemLocale(systemLocale());
}

/** The language a State asks for; gui.json omits uiLanguage when it follows the system */
export const prefOf = (state: State): LangPref => state.prefs.uiLanguage ?? '';

export interface StateHandlers {
  /** The newest State: a push, or the first snapshot unless a push came before it */
  onState(state: State): void;
  /** The first snapshot can't be read and no push has come: App asks again and shows the failure */
  onFailure(): void;
}

/**
 * Subscribes to the pushes before asking for the first snapshot, so nothing is
 * missed. A push always wins over a snapshot that resolves after it.
 */
export function watchState(api: Api, handlers: StateHandlers): Unsubscribe {
  let alive = true;
  let pushed = false;
  const off = api.onState((state) => {
    if (alive) {
      pushed = true;
      handlers.onState(state);
    }
  });
  api.state().then(
    (state) => {
      if (alive && !pushed) handlers.onState(state);
    },
    () => {
      if (alive && !pushed) handlers.onFailure();
    },
  );
  return () => {
    alive = false;
    off();
  };
}

/**
 * Gives the window its language before anything renders: nothing shows until a
 * State is known, so the saved language never flashes past the system's. It keeps
 * the newest State, which App takes as its first one.
 */
export function Root({ api }: { api: Api }) {
  // undefined until a State is known; null when the first snapshot can't be read
  const [latest, setLatest] = useState<State | null | undefined>(undefined);
  useEffect(() => watchState(api, { onState: setLatest, onFailure: () => setLatest(null) }), [api]);
  useEffect(() => reportSystemLocale(api), [api]);
  const persist = useCallback((next: LangPref) => api.setUILanguage(next), [api]);

  if (latest === undefined) return null;
  return (
    <I18nProvider pref={latest ? prefOf(latest) : ''} persist={persist}>
      <App api={api} initial={latest ?? undefined} />
    </I18nProvider>
  );
}
