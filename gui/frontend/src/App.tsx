import { useState } from 'react';
import type { Api } from './api';
import { ChapterPanel } from './ChapterPanel';
import { errorMessage } from './format';
import { LogPanel, ActionBar, ProgressPanel } from './RunPanel';
import { OptionsPanel } from './OptionsPanel';
import type { Options } from './types';
import { useWindow } from './useWindow';

export function App({ api }: { api: Api }) {
  const { state, chapters, selected, log } = useWindow(api);
  const [url, setUrl] = useState<string | null>(null);
  const [actionError, setActionError] = useState('');
  const [optionError, setOptionError] = useState('');

  if (state === null) {
    return <main className="app">Loading…</main>;
  }

  const shownUrl = url ?? state.url;
  const busy = state.loading || state.running;

  const load = (target: string) => {
    setActionError('');
    api.load(target).catch((e) => setActionError(errorMessage(e)));
  };

  return (
    <main className="app">
      <header className="top">
        <h1>Manga Downloader</h1>
        <form
          className="url"
          onSubmit={(e) => {
            e.preventDefault();
            load(shownUrl);
          }}
        >
          <input
            type="url"
            aria-label="Series URL"
            placeholder="https://… (the series page)"
            value={shownUrl}
            disabled={busy}
            onChange={(e) => setUrl(e.target.value)}
          />
          <button type="submit" disabled={busy || shownUrl.trim() === ''}>
            {state.loading ? 'Loading…' : 'Load'}
          </button>
        </form>
        {state.loadError !== '' && <p className="error">{state.loadError}</p>}
        {state.stale && <p className="notice">The language or scanlator changed since the list was loaded.</p>}
      </header>

      <div className="layout">
        <ChapterPanel
          state={state}
          chapters={chapters}
          selected={selected}
          disabled={busy || !state.loaded}
          onToggle={(index, on) => api.setSelected(index, on)}
          onSetAll={(on) => api.setAll(on)}
          onRange={(text) => api.selectRange(text)}
          onError={setActionError}
        />

        <div className="side">
          <OptionsPanel
            options={state.options}
            state={state}
            disabled={busy}
            onCommit={(o: Options) => {
              setOptionError('');
              return api.setOptions(o);
            }}
            onChooseFolder={async () => {
              setOptionError('');
              await api.chooseFolder();
            }}
            onReload={() => load(state.url)}
            onError={setOptionError}
          />
          {optionError !== '' && <p className="error">{optionError}</p>}
          <ActionBar
            state={state}
            actionError={actionError}
            onStart={() => {
              setActionError('');
              api.start().catch((e) => setActionError(errorMessage(e)));
            }}
            onCancel={() => {
              api.cancel().catch((e) => setActionError(errorMessage(e)));
            }}
            onOpenFolder={() => {
              api.openFolder().catch((e) => setActionError(errorMessage(e)));
            }}
          />
        </div>
      </div>

      <ProgressPanel state={state} />
      <LogPanel log={log} />
      <footer className="foot">{state.version}</footer>
    </main>
  );
}
