// The foot of the side panel, always in view: the one big action. Download
// when idle (with the reason it can't start, if any), the run's overall
// progress and Cancel while it runs.

import { useEffect, useMemo, useRef } from 'react';
import type { T as Translator } from './i18n';
import { T, useI18n } from './i18n/react';
import { DownloadIcon, Spinner, XIcon } from './icons';
import { downloadBlocker, downloadLabel, overallProgress } from './run';
import type { Chapter, Options, PackAs, State } from './types';

interface Props {
  state: State;
  /** The backend's refusal of the last Download or Cancel */
  error: string;
  /** The loaded chapters and the selected indices, for the Download label's volumes */
  chapters: Chapter[];
  selected: number[];
  onStart: () => void;
  onCancel: () => void;
}

export function DownloadDock({ state, error, chapters, selected, onStart, onCancel }: Props) {
  const { t } = useI18n();
  // Download and Cancel are different buttons, so the focus moves to the other
  // one when the run starts or ends, rather than falling to the body
  const focused = useRef<'start' | 'cancel' | null>(null);
  const startButton = useRef<HTMLButtonElement>(null);
  const cancelButton = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (state.running && focused.current === 'start') {
      cancelButton.current?.focus();
    } else if (!state.running && focused.current === 'cancel') {
      if (startButton.current?.disabled) {
        // the run ended with nothing to download: focus the chapter list instead of the body
        document.querySelector<HTMLElement>('[role="listbox"]')?.focus();
      } else {
        startButton.current?.focus();
      }
    }
  }, [state.running]);

  // the label regroups the chapters, so it's kept across the progress ticks
  const label = useMemo(
    () => downloadLabel(state, chapters, selected, t),
    [state.selectedCount, state.options.packAs, state.volumes.text, chapters, selected, t],
  );

  if (state.running) {
    const overall = overallProgress(state);
    const pct = Math.round(overall.fraction * 100);
    return (
      <div className="dock running">
        <div className="dock-progress">
          <div className="dock-line">
            <span className="dock-title">
              <Spinner />
              {state.cancelling
                ? t('download.progress.finishing')
                : state.run.bundle
                  ? t('download.progress.bundle')
                  : state.run.volumes
                    ? t('download.progress.files', { done: overall.finished, total: overall.total })
                    : t('download.progress.chapters', { done: overall.finished, total: overall.total })}
            </span>
            <span className="dock-pct">{pct}%</span>
          </div>
          <div
            className="bar big"
            role="progressbar"
            aria-label={t('download.progress.label')}
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={pct}
          >
            <span style={{ transform: `scaleX(${overall.fraction})` }} />
          </div>
          {state.cancelling && <p className="hint">{t('download.progress.cancelHint')}</p>}
        </div>
        {/* aria-disabled, not disabled: a disabled button drops its focus, and Cancel's focus must survive to the run's end */}
        <button
          type="button"
          ref={cancelButton}
          className="btn danger-outline block"
          aria-disabled={state.cancelling || undefined}
          onFocus={() => (focused.current = 'cancel')}
          onBlur={() => (focused.current = null)}
          onClick={() => {
            if (!state.cancelling) {
              onCancel();
            }
          }}
        >
          <XIcon /> {state.cancelling ? t('download.cancelling') : t('common.cancel')}
        </button>
        {error !== '' && (
          <p className="error-text small" role="alert">
            {error}
          </p>
        )}
      </div>
    );
  }

  const blocker = downloadBlocker(state, t);
  const o = state.options;
  return (
    <div className="dock">
      <p className="dock-dest" title={o.outputDir}>
        <T
          k="download.where"
          values={{ what: destination(o, t), dir: o.outputDir || '…' }}
          components={{
            dir: (children) => (
              <span className="mono">
                <bdi>{children}</bdi>
              </span>
            ),
          }}
        />
      </p>
      <button
        type="button"
        ref={startButton}
        className="btn primary block big"
        disabled={blocker !== ''}
        aria-describedby={blocker !== '' ? 'dock-blocker' : undefined}
        onFocus={() => (focused.current = 'start')}
        onBlur={() => (focused.current = null)}
        onClick={onStart}
      >
        <DownloadIcon /> {label}
      </button>
      {error !== '' ? (
        <p className="error-text small" role="alert">
          {t('download.failed', { error })}
        </p>
      ) : (
        blocker !== '' && (
          <p className="hint center" id="dock-blocker">
            {blocker}
          </p>
        )
      )}
    </div>
  );
}

/** What the run writes, by format and Pack as, as the dock's first words */
function destinations(t: Translator): Record<Options['format'], Record<PackAs, string>> {
  const cbz = t('download.dest.cbzFiles');
  const cbr = t('download.dest.cbrFiles');
  const epub = t('download.dest.epubFiles');
  const raw = t('download.dest.rawFiles');
  return {
    cbz: { chapters: cbz, parts: cbz, volumes: t('download.dest.cbzPerVolume'), bundle: t('download.dest.cbzBundle') },
    cbr: { chapters: cbr, parts: cbr, volumes: t('download.dest.cbrPerVolume'), bundle: t('download.dest.cbrBundle') },
    epub: {
      chapters: epub,
      parts: epub,
      volumes: t('download.dest.epubPerVolume'),
      bundle: t('download.dest.epubBundle'),
    },
    raw: {
      chapters: raw,
      parts: raw,
      volumes: t('download.dest.rawPerVolume'),
      bundle: t('download.dest.rawBundle'),
    },
  };
}

/** What the run writes, as the dock's first words */
function destination(o: Options, t: Translator): string {
  const byPack = destinations(t)[o.format];
  return byPack?.[o.packAs] ?? byPack?.chapters ?? t('download.dest.files');
}
