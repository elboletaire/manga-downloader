// The window. Three bands, top to bottom, following the job's own order:
//
//   source bar  what to fetch (URL, language and scanlator filters)
//   workspace   the chapter list | a side panel with Options and Activity,
//               and the Download action pinned under both tabs
//   status bar  the latest log line (the full log opens as a drawer), version

import { useEffect, useRef, useState } from 'react';
import { ActivityPanel } from './ActivityPanel';
import type { Api } from './api';
import { BrowserAlert } from './BrowserAlert';
import { ChapterPanel } from './ChapterPanel';
import { DownloadDock } from './DownloadDock';
import { errorMessage } from './format';
import { ActivityIcon, SlidersIcon, Spinner } from './icons';
import { useI18n } from './i18n/react';
import { Mascot } from './Mascot';
import { LogDrawer, StatusBar } from './LogPanel';
import { createOptionsWriter } from './optionsWriter';
import { OptionsPanel } from './OptionsPanel';
import { SourceBar } from './SourceBar';
import { tally } from './run';
import type { Options } from './types';
import { useWindow } from './useWindow';

type Tab = 'options' | 'activity';

export function App({ api }: { api: Api }) {
  const { t } = useI18n();
  const { state, chapters, selected, log, failure } = useWindow(api);
  const [tab, setTab] = useState<Tab>('options');
  const [logOpen, setLogOpen] = useState(false);
  const [loadRefusal, setLoadRefusal] = useState('');
  const [actionError, setActionError] = useState('');
  const [optionError, setOptionError] = useState('');
  // options are written one at a time, each patch over the last set sent
  const [optionsWriter] = useState(() =>
    createOptionsWriter((options, load) =>
      load === undefined ? api.setOptions(options) : api.setOptionsAndLoad(options, load),
    ),
  );

  useEffect(() => {
    if (state !== null) {
      optionsWriter.pushed(state.options);
    }
  }, [state, optionsWriter]);

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
        <Mascot expression="soft" size={48} />
        {failure !== null ? (
          <p className="error-text" role="alert">
            {t('app.readFailed', { error: failure })}
          </p>
        ) : (
          <p className="muted">
            <Spinner /> {t('app.starting')}
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
  const clear = () => {
    setLoadRefusal('');
    api.clear().catch((e) => setLoadRefusal(errorMessage(e)));
  };
  const setOptions = (patch: Partial<Options>) => {
    setOptionError('');
    return optionsWriter.patch(patch).catch((e: unknown) => {
      setOptionError(errorMessage(e));
      throw e;
    });
  };
  // a filter picked from a selector applies at once: it's written with the
  // reload of the loaded series, so it's never shown as not applied
  const pickFilter = (patch: Partial<Options>) => {
    setOptionError('');
    return optionsWriter.patch(patch, state.url).catch((e: unknown) => {
      setOptionError(errorMessage(e));
    });
  };
  const counts = tally(state.jobs);
  const failed = state.summary && !state.running ? state.summary.failures.length : counts.failed;

  return (
    <div className="app">
      <SourceBar
        state={state}
        onLoad={load}
        readClipboard={() => api.readClipboard()}
        onClear={clear}
        onOptions={setOptions}
        onPick={pickFilter}
        onError={setOptionError}
        loadRefusal={loadRefusal}
        optionError={optionError}
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
          onVolumes={(text) => api.setVolumes(text)}
          onReload={() => load(state.url)}
          onError={setActionError}
        />

        <aside className="side" aria-label={t('app.download')}>
          <div className="tabs" role="tablist" aria-label={t('app.tabs.label')}>
            <TabButton id="options" tab={tab} onSelect={setTab}>
              <SlidersIcon size={15} /> {t('app.tabs.options')}
            </TabButton>
            <TabButton id="activity" tab={tab} onSelect={setTab}>
              <ActivityIcon size={15} /> {t('app.tabs.activity')}
              {state.running ? (
                <span className="tab-badge live" aria-label={t('app.running')}>
                  <Spinner size={11} />
                </span>
              ) : failed > 0 ? (
                <span className="tab-badge danger" aria-label={t('count.failed', { count: failed })}>
                  {failed}
                </span>
              ) : state.summary ? (
                <span className="tab-badge success" aria-label={t('app.finished')} />
              ) : null}
            </TabButton>
          </div>

          {/* both panels stay mounted, so each tab's aria-controls has a target */}
          <div
            className="side-body"
            role="tabpanel"
            id="panel-options"
            aria-labelledby="tab-options"
            tabIndex={0}
            hidden={tab !== 'options'}
          >
            <OptionsPanel
              state={state}
              onOptions={setOptions}
              onChooseFolder={async () => {
                setOptionError('');
                await api.chooseFolder();
              }}
              onError={setOptionError}
            />
          </div>
          <div
            className="side-body"
            role="tabpanel"
            id="panel-activity"
            aria-labelledby="tab-activity"
            tabIndex={0}
            hidden={tab !== 'activity'}
          >
            <ActivityPanel
              state={state}
              onOpenFolder={() => {
                api.openFolder().catch((e) => setActionError(errorMessage(e)));
              }}
            />
          </div>

          <DownloadDock
            state={state}
            error={actionError}
            chapters={chapters}
            selected={selected}
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
      {state.browserAlert !== '' && (
        <BrowserAlert
          message={state.browserAlert}
          onShow={() => {
            api.showBrowser().catch(() => undefined);
          }}
          onDismiss={() => {
            api.dismissBrowserAlert().catch(() => undefined);
          }}
        />
      )}
      <StatusBar
        log={log}
        version={state.version}
        releaseURL={state.releaseURL}
        update={state.update}
        open={logOpen}
        onToggle={() => setLogOpen((o) => !o)}
        onOpenURL={(url) => api.openURL(url)}
        onUpdate={() => api.update()}
      />
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
