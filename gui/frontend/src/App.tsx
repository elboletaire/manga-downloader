// The window. Three bands, top to bottom, following the job's own order:
//
//   source bar  what to fetch (URL, language and scanlator filters)
//   workspace   the chapter list | a side panel with Options and Activity,
//               and the Download action pinned under both tabs
//   status bar  the latest log line (the full log opens as a drawer), version

import { useEffect, useRef, useState } from 'react';
import { ActivityPanel } from './ActivityPanel';
import type { Api } from './api';
import { ChapterPanel } from './ChapterPanel';
import { DownloadDock } from './DownloadDock';
import { errorMessage } from './format';
import { ActivityIcon, Logo, SlidersIcon, Spinner } from './icons';
import { LogDrawer, StatusBar } from './LogPanel';
import { OptionsPanel } from './OptionsPanel';
import { SourceBar } from './SourceBar';
import { tally } from './run';
import type { Options } from './types';
import { useWindow } from './useWindow';

type Tab = 'options' | 'activity';

export function App({ api }: { api: Api }) {
  const { state, chapters, selected, log, failure } = useWindow(api);
  const [tab, setTab] = useState<Tab>('options');
  const [logOpen, setLogOpen] = useState(false);
  const [loadRefusal, setLoadRefusal] = useState('');
  const [actionError, setActionError] = useState('');
  const [optionError, setOptionError] = useState('');

  // a run that starts brings its progress into view, as does opening the
  // window on a run (or its outcome); the tab stays the user's to change
  const running = state?.running ?? false;
  const wasRunning = useRef<boolean | null>(null);
  useEffect(() => {
    if (state === null) {
      return;
    }
    const first = wasRunning.current === null;
    if ((running && !wasRunning.current) || (first && state.summary !== null)) {
      setTab('activity');
    }
    wasRunning.current = running;
  }, [running, state === null]);

  if (state === null) {
    return (
      <main className="boot">
        <Logo size={40} />
        {failure !== '' ? (
          <p className="error-text" role="alert">
            {failure}
          </p>
        ) : (
          <p className="muted">
            <Spinner /> Starting…
          </p>
        )}
      </main>
    );
  }

  const busy = state.loading || state.running;

  const load = (target: string) => {
    setLoadRefusal('');
    setActionError('');
    api.load(target).catch((e) => setLoadRefusal(errorMessage(e)));
  };
  const setOptions = (patch: Partial<Options>) => {
    setOptionError('');
    return api.setOptions({ ...state.options, ...patch }).catch((e: unknown) => {
      setOptionError(errorMessage(e));
      throw e;
    });
  };
  const counts = tally(state.jobs);
  const failed = state.summary && !state.running ? state.summary.failures.length : counts.failed;

  return (
    <div className="app">
      <SourceBar
        state={state}
        onLoad={load}
        onOptions={setOptions}
        onError={setOptionError}
        loadRefusal={loadRefusal}
      />

      <main className="workspace">
        <ChapterPanel
          state={state}
          chapters={chapters}
          selected={selected}
          disabled={busy}
          onToggle={(index, on) => api.setSelected(index, on)}
          onToggleMany={(indices, on) => api.setSelectedMany(indices, on)}
          onSetAll={(on) => api.setAll(on)}
          onRange={(text) => api.selectRange(text)}
          onReload={() => load(state.url)}
          onError={setActionError}
        />

        <aside className="side" aria-label="Download">
          <div className="tabs" role="tablist" aria-label="Side panel">
            <TabButton id="options" tab={tab} onSelect={setTab}>
              <SlidersIcon size={15} /> Options
            </TabButton>
            <TabButton id="activity" tab={tab} onSelect={setTab}>
              <ActivityIcon size={15} /> Activity
              {state.running ? (
                <span className="tab-badge live" aria-label="running">
                  <Spinner size={11} />
                </span>
              ) : failed > 0 ? (
                <span className="tab-badge danger" aria-label={`${failed} failed`}>
                  {failed}
                </span>
              ) : state.summary ? (
                <span className="tab-badge success" aria-label="finished" />
              ) : null}
            </TabButton>
          </div>

          <div className="side-body" role="tabpanel" id={`panel-${tab}`} aria-labelledby={`tab-${tab}`}>
            {tab === 'options' ? (
              <OptionsPanel
                state={state}
                onOptions={setOptions}
                onChooseFolder={async () => {
                  setOptionError('');
                  await api.chooseFolder();
                }}
                onError={setOptionError}
                error={optionError}
              />
            ) : (
              <ActivityPanel
                state={state}
                onOpenFolder={() => {
                  api.openFolder().catch((e) => setActionError(errorMessage(e)));
                }}
              />
            )}
          </div>

          <DownloadDock
            state={state}
            error={actionError}
            onStart={() => {
              setActionError('');
              api.start().catch((e) => setActionError(errorMessage(e)));
            }}
            onCancel={() => {
              api.cancel().catch((e) => setActionError(errorMessage(e)));
            }}
          />
        </aside>
      </main>

      {logOpen && <LogDrawer log={log} onClose={() => setLogOpen(false)} />}
      <StatusBar log={log} version={state.version} open={logOpen} onToggle={() => setLogOpen((o) => !o)} />
    </div>
  );
}

function TabButton({
  id,
  tab,
  onSelect,
  children,
}: {
  id: Tab;
  tab: Tab;
  onSelect: (t: Tab) => void;
  children: React.ReactNode;
}) {
  const on = id === tab;
  return (
    <button
      type="button"
      role="tab"
      id={`tab-${id}`}
      aria-selected={on}
      aria-controls={`panel-${id}`}
      tabIndex={on ? 0 : -1}
      className={`tab${on ? ' on' : ''}`}
      onClick={() => onSelect(id)}
      onKeyDown={(e) => {
        if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
          e.preventDefault();
          const next: Tab = id === 'options' ? 'activity' : 'options';
          onSelect(next);
          document.getElementById(`tab-${next}`)?.focus();
        }
      }}
    >
      {children}
    </button>
  );
}
