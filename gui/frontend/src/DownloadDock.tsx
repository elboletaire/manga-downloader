// The foot of the side panel, always in view: the one big action. Download
// when idle (with the reason it can't start, if any), the run's overall
// progress and Cancel while it runs.

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
        <button type="button" className="btn danger-outline block" disabled={state.cancelling} onClick={onCancel}>
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
        className="btn primary block big"
        disabled={blocker !== ''}
        aria-describedby={blocker !== '' ? 'dock-blocker' : undefined}
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
