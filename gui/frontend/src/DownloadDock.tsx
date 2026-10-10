// The foot of the side panel, always in view: the one big action. Download
// when idle (with the reason it can't start, if any), the run's overall
// progress and Cancel while it runs.

import { useEffect, useRef } from 'react';
import { DownloadIcon, Spinner, XIcon } from './icons';
import { downloadBlocker, downloadLabel, overallProgress } from './run';
import type { State } from './types';

interface Props {
  state: State;
  /** The backend's refusal of the last Download or Cancel */
  error: string;
  onStart: () => void;
  onCancel: () => void;
}

export function DownloadDock({ state, error, onStart, onCancel }: Props) {
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
                ? 'Finishing the current chapters'
                : state.run.bundle
                  ? 'Building the bundle'
                  : `${overall.finished} of ${overall.total} chapters done`}
            </span>
            <span className="dock-pct">{pct}%</span>
          </div>
          <div
            className="bar big"
            role="progressbar"
            aria-label="Overall progress"
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={pct}
          >
            <span style={{ transform: `scaleX(${overall.fraction})` }} />
          </div>
          {state.cancelling && <p className="hint">Chapters already downloading will finish; no new ones start.</p>}
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
          <XIcon /> {state.cancelling ? 'Cancelling…' : 'Cancel'}
        </button>
        {error !== '' && (
          <p className="error-text small" role="alert">
            {error}
          </p>
        )}
      </div>
    );
  }

  const blocker = downloadBlocker(state);
  const o = state.options;
  return (
    <div className="dock">
      <p className="dock-dest" title={o.outputDir}>
        {o.format === 'cbz' ? (o.bundle ? 'One CBZ' : 'CBZ files') : 'Image folders'} in{' '}
        <span className="mono">
          <bdi>{o.outputDir || '…'}</bdi>
        </span>
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
        <DownloadIcon /> {downloadLabel(state)}
      </button>
      {error !== '' ? (
        <p className="error-text small" role="alert">
          Can't download: {error}
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
