import { useEffect, useRef } from 'react';
import { jobFraction, jobStatus, summaryHeadline, summaryLines } from './format';
import type { LogLine, State } from './types';

interface ActionProps {
  state: State;
  actionError: string;
  onStart: () => void;
  onCancel: () => void;
  onOpenFolder: () => void;
}

/** Download and Cancel, the refusal of the last Download, and the last run's outcome */
export function ActionBar({ state, actionError, onStart, onCancel, onOpenFolder }: ActionProps) {
  const busy = state.loading || state.running;
  const summary = state.summary;

  return (
    <section className="panel actions">
      <h2>Download</h2>
      <div className="buttons">
        <button type="button" className="primary" disabled={!state.loaded || busy} onClick={onStart}>
          Download
        </button>
        <button type="button" disabled={!state.running || state.cancelling} onClick={onCancel}>
          {state.cancelling ? 'Cancelling…' : 'Cancel'}
        </button>
      </div>
      {actionError !== '' && <p className="error">Can't download: {actionError}</p>}
      {state.running && (
        <p className="status">
          Downloading {state.run.chapters} {state.run.chapters === 1 ? 'chapter' : 'chapters'}…
        </p>
      )}
      {summary && !state.running && (
        <div className="summary">
          <p className="headline">{summaryHeadline(summary)}</p>
          <div className="summary-text">
            {summaryLines(summary).map((line, i) => (
              <p key={i}>{line}</p>
            ))}
          </div>
          <button type="button" onClick={onOpenFolder}>
            Open folder
          </button>
        </div>
      )}
    </section>
  );
}

interface ProgressProps {
  state: State;
}

export function ProgressPanel({ state }: ProgressProps) {
  if (state.jobs.length === 0) {
    return (
      <section className="panel progress">
        <h2>Progress</h2>
        <p className="hint">Select chapters and press Download to follow them here.</p>
      </section>
    );
  }

  return (
    <section className="panel progress">
      <h2>Progress</h2>
      <ul className="jobs">
        {state.jobs.map((j, i) => (
          <li key={`${j.number}-${i}`} className={`job job-${j.state}`}>
            <div className="job-head">
              <span className="job-title">{j.title}</span>
              <span className="job-status">{jobStatus(j)}</span>
            </div>
            <progress max={1} value={jobFraction(j)} aria-label={`Progress of ${j.title}`} />
          </li>
        ))}
      </ul>
    </section>
  );
}

interface LogProps {
  log: LogLine[];
}

export function LogPanel({ log }: LogProps) {
  const box = useRef<HTMLDivElement>(null);

  // follow the newest line, unless the reader has scrolled up to read older ones
  useEffect(() => {
    const el = box.current;
    if (el && el.scrollHeight - el.scrollTop - el.clientHeight < 60) {
      el.scrollTop = el.scrollHeight;
    }
  }, [log]);

  return (
    <section className="panel log">
      <h2>Log</h2>
      <div className="log-lines" ref={box} role="log">
        {log.length === 0 && <p className="hint">Nothing logged yet.</p>}
        {log.map((line, i) => (
          <p key={i} className={`line line-${line.level}`}>
            {line.text}
          </p>
        ))}
      </div>
    </section>
  );
}
