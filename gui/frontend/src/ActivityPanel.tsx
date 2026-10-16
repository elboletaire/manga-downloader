// What the run is doing and how the last one ended: the summary on top, then
// one row per chapter (or the single bundle job). Rows are windowed like the
// chapter list, since "select all" on a long series makes thousands of them.

import { jobFraction, jobShortStatus, jobStatus, plural, summaryHeadline, summarySentence } from './format';
import { AlertIcon, CheckIcon, ErrorIcon, FolderIcon, PackageIcon, SkipIcon, Spinner, XIcon } from './icons';
import { overallProgress, splitFailure, summaryTone } from './run';
import type { Job, State, Summary } from './types';
import { useVirtualList } from './virtual';

const JOB_HEIGHT = 50;

interface Props {
  state: State;
  onOpenFolder: () => void;
}

export function ActivityPanel({ state, onOpenFolder }: Props) {
  const summary = state.running ? null : state.summary;

  if (state.jobs.length === 0 && summary === null) {
    return (
      <div className="activity empty-activity">
        <p className="muted">Nothing downloaded yet.</p>
        <p className="muted small">
          Pick chapters and press Download: each one shows up here as it's fetched and packed.
        </p>
      </div>
    );
  }

  return (
    <div className="activity">
      {summary && <SummaryCard summary={summary} onOpenFolder={onOpenFolder} />}
      {state.jobs.length > 0 &&
        (state.run.bundle ? (
          <BundleCard job={state.jobs[0]} chapters={state.run.chapters} />
        ) : (
          <JobList state={state} />
        ))}
    </div>
  );
}

function SummaryCard({ summary, onOpenFolder }: { summary: Summary; onOpenFolder: () => void }) {
  const tone = summaryTone(summary);
  const ToneIcon =
    tone === 'success' ? CheckIcon : tone === 'neutral' ? XIcon : tone === 'warning' ? AlertIcon : ErrorIcon;

  return (
    <section className={`summary tone-${tone}`} aria-label="Last download">
      <div className="summary-head">
        <span className="summary-icon">
          <ToneIcon size={16} />
        </span>
        <h3>{summaryHeadline(summary)}</h3>
      </div>
      <p className="summary-text">{summarySentence(summary)}</p>
      {summary.error !== '' && <p className="error-text small">{summary.error}</p>}
      {summary.skipped > 0 && (
        <p className="muted small">
          {plural(summary.skipped, 'chapter was', 'chapters were')} skipped: the skip-pages option excluded every page.
        </p>
      )}
      {summary.failures.length > 0 && (
        <ul className="failures" aria-label="Failed chapters">
          {summary.failures.map((f, i) => {
            const { what, why } = splitFailure(f);
            return (
              <li key={i}>
                {what !== '' && <strong>{what}</strong>}
                <span>{why}</span>
              </li>
            );
          })}
        </ul>
      )}
      <button type="button" className="btn small" onClick={onOpenFolder}>
        <FolderIcon size={14} /> Open folder
      </button>
    </section>
  );
}

/** A bundle is one job; its phase text is the story, so it gets room to show it */
function BundleCard({ job, chapters }: { job: Job; chapters: number }) {
  const fraction = jobFraction(job);
  const moving = job.state === 'downloading' || job.state === 'archiving';
  return (
    <section className={`bundle-card job-${job.state}`} aria-label="Bundle">
      <div className="job-line">
        <span className="job-title">{job.title}</span>
        <span className="job-status">{plural(chapters, 'chapter', 'chapters')}</span>
      </div>
      <p className="bundle-phase">
        <JobIcon state={job.state} />
        <span>{moving ? jobShortStatus(job) : jobStatus(job)}</span>
      </p>
      {moving && (
        <div
          className="bar"
          role="progressbar"
          aria-label="Progress of the bundle"
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={Math.round(fraction * 100)}
        >
          <span style={{ transform: `scaleX(${fraction})` }} />
        </div>
      )}
    </section>
  );
}

function JobList({ state }: { state: State }) {
  const overall = overallProgress(state);
  const queued = state.running ? overall.queued : 0;
  const rows = state.jobs.length + (queued > 0 ? 1 : 0);
  const list = useVirtualList(rows, JOB_HEIGHT, 4);
  const { first, last } = list.span;

  return (
    <section className="jobs-section" aria-label="Chapters in this run">
      <h3 className="jobs-title">
        {state.running ? 'This run' : 'Last run'}
        <span className="muted">
          {' '}
          · {overall.finished} of {overall.total} finished
        </span>
      </h3>
      <div className="jobs" ref={list.ref} onScroll={list.onScroll} role="list">
        <div className="list-inner" style={{ height: rows * JOB_HEIGHT }}>
          {Array.from({ length: last - first }, (_, i) => {
            const pos = first + i;
            const style = { transform: `translateY(${pos * JOB_HEIGHT}px)`, height: JOB_HEIGHT };
            const job = state.jobs[pos];
            if (job === undefined) {
              return (
                <div key="queued" role="listitem" className="job job-queued" style={style}>
                  <span className="job-icon" aria-hidden="true" />
                  <span className="job-title muted">{plural(queued, 'more chapter', 'more chapters')} waiting</span>
                </div>
              );
            }
            return <JobRow key={pos} job={job} style={style} />;
          })}
        </div>
      </div>
    </section>
  );
}

function JobRow({ job, style }: { job: Job; style: React.CSSProperties }) {
  const fraction = jobFraction(job);
  const status = jobStatus(job);

  return (
    <div role="listitem" className={`job job-${job.state}`} style={style} title={status}>
      <span className="job-icon" aria-hidden="true">
        <JobIcon state={job.state} />
      </span>
      <div className="job-body">
        <div className="job-line">
          <span className="job-title">{job.title}</span>
          {job.tags && <span className="job-tags">{job.tags}</span>}
          <span className="job-status">{jobShortStatus(job)}</span>
        </div>
        {job.state === 'failed' || job.state === 'skipped' ? (
          <span className="job-reason">{job.reason}</span>
        ) : (
          <div
            className="bar"
            role="progressbar"
            aria-label={`Progress of ${job.title}${job.tags}`}
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={Math.round(fraction * 100)}
          >
            <span style={{ transform: `scaleX(${fraction})` }} />
          </div>
        )}
      </div>
    </div>
  );
}

function JobIcon({ state }: { state: Job['state'] }) {
  switch (state) {
    case 'fetching':
    case 'downloading':
      return <Spinner size={14} />;
    case 'archiving':
      return <PackageIcon size={15} />;
    case 'done':
      return <CheckIcon size={15} />;
    case 'failed':
      return <ErrorIcon size={15} />;
    case 'skipped':
      return <SkipIcon size={15} />;
  }
}
