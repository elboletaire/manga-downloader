// The log lives in the status bar: its newest line is always in view, and the
// whole of it opens as a drawer over the window. Warnings and errors are
// counted on the toggle, so a problem isn't missed with the drawer closed.

import { useEffect, useRef, useState, type MouseEvent } from 'react';
import { AlertIcon, CopyIcon, ErrorIcon, LogIcon, XIcon } from './icons';
import { UpdateDialog } from './UpdateDialog';
import type { LogLine, Update } from './types';

interface BarProps {
  log: LogLine[];
  version: string;
  /** The GitHub release of this build; "" for a dev build, which has none */
  releaseURL: string;
  /** A newer release, when the update check found one */
  update: Update | null;
  open: boolean;
  onToggle: () => void;
  /** Opens a link in the system's browser */
  onOpenURL: (url: string) => void;
  /** Installs the update (update.oneClick); rejects when it can't, the backend having logged why */
  onUpdate: () => Promise<void>;
}

export function StatusBar({ log, version, releaseURL, update, open, onToggle, onOpenURL, onUpdate }: BarProps) {
  const [updateOpen, setUpdateOpen] = useState(update?.prompt === true);

  // links open outside the window, which must stay on the app
  const openLink = (url: string) => (e: MouseEvent<HTMLAnchorElement>) => {
    e.preventDefault();
    onOpenURL(url);
  };

  const lastLine = log.length > 0 ? log[log.length - 1] : undefined;
  let warns = 0;
  let errors = 0;
  for (const l of log) {
    if (l.level === 'warn') {
      warns++;
    } else if (l.level === 'error') {
      errors++;
    }
  }

  return (
    <footer className="statusbar">
      <button
        type="button"
        className={`log-toggle${open ? ' open' : ''}`}
        aria-expanded={open}
        aria-controls="log-drawer"
        onClick={onToggle}
        title={open ? 'Hide the log' : 'Show the log'}
      >
        <LogIcon size={14} />
        <span>Log</span>
        {errors > 0 && (
          <span className="count-badge danger" aria-label={`${errors} errors`}>
            {errors}
          </span>
        )}
        {warns > 0 && (
          <span className="count-badge warning" aria-label={`${warns} warnings`}>
            {warns}
          </span>
        )}
      </button>
      <span className={`last-line level-${lastLine?.level ?? 'info'}`} aria-live="polite" title={lastLine?.text}>
        {lastLine ? lastLine.text : 'Ready'}
      </span>
      <span className="version-group">
        {update && (
          <a
            className="update-badge warning"
            href={update.url}
            onClick={
              update.oneClick
                ? (e) => {
                    e.preventDefault();
                    setUpdateOpen(true);
                  }
                : openLink(update.url)
            }
            title={`Update available: ${update.latest}`}
            aria-label={`Update available: ${update.latest}`}
          >
            <svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true" focusable="false">
              <circle className="update-disc" cx="8" cy="8" r="7" strokeWidth="1.5" />
              <line x1="8" y1="4.6" x2="8" y2="8.6" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
              <circle cx="8" cy="11.2" r="1.1" fill="currentColor" stroke="none" />
            </svg>
          </a>
        )}
        {releaseURL !== '' ? (
          <a className="version" href={releaseURL} onClick={openLink(releaseURL)} title="This version's release">
            {version}
          </a>
        ) : (
          <span className="version" title="Version">
            {version}
          </span>
        )}
      </span>
      {update?.oneClick && updateOpen && (
        <UpdateDialog
          latest={update.latest}
          current={version}
          onUpdate={onUpdate}
          onReleaseNotes={() => onOpenURL(update.url)}
          onClose={() => setUpdateOpen(false)}
        />
      )}
    </footer>
  );
}

interface DrawerProps {
  log: LogLine[];
  onClose: () => void;
}

export function LogDrawer({ log, onClose }: DrawerProps) {
  const box = useRef<HTMLDivElement>(null);
  const [copied, setCopied] = useState(false);

  // follow the newest line, unless the reader has scrolled up to read older ones
  useEffect(() => {
    const el = box.current;
    if (el && el.scrollHeight - el.scrollTop - el.clientHeight < 60) {
      el.scrollTop = el.scrollHeight;
    }
  }, [log]);

  // opening the drawer starts at the bottom
  useEffect(() => {
    const el = box.current;
    if (el) {
      el.scrollTop = el.scrollHeight;
      el.focus();
    }
  }, []);

  const copy = () => {
    const text = log.map((l) => (l.level === 'info' ? l.text : `${l.level.toUpperCase()}: ${l.text}`)).join('\n');
    navigator.clipboard?.writeText(text).then(
      () => {
        setCopied(true);
        setTimeout(() => setCopied(false), 1500);
      },
      () => undefined,
    );
  };

  return (
    <section
      className="log-drawer"
      id="log-drawer"
      aria-label="Log"
      onKeyDown={(e) => {
        if (e.key === 'Escape') {
          onClose();
        }
      }}
    >
      <div className="log-head">
        <h2>Log</h2>
        <button type="button" className="btn ghost small" onClick={copy} disabled={log.length === 0}>
          <CopyIcon size={14} /> {copied ? 'Copied' : 'Copy'}
        </button>
        <button type="button" className="btn ghost small icon-only" aria-label="Close the log" onClick={onClose}>
          <XIcon size={14} />
        </button>
      </div>
      <div className="log-lines" ref={box} role="log" tabIndex={0}>
        {log.length === 0 && <p className="muted">Nothing logged yet.</p>}
        {log.map((line, i) => (
          <p key={i} className={`line level-${line.level}`}>
            {line.level === 'error' ? <ErrorIcon size={13} /> : line.level === 'warn' ? <AlertIcon size={13} /> : null}
            <span>{line.text}</span>
          </p>
        ))}
      </div>
    </section>
  );
}
