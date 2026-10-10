// The log lives in the status bar: its newest line is always in view, and the
// whole of it opens as a drawer over the window. Warnings and errors are
// counted on the toggle, so a problem isn't missed with the drawer closed.

import { useEffect, useRef, useState } from 'react';
import { AlertIcon, CopyIcon, ErrorIcon, LogIcon, XIcon } from './icons';
import type { LogLine } from './types';

interface BarProps {
  log: LogLine[];
  version: string;
  open: boolean;
  onToggle: () => void;
}

export function StatusBar({ log, version, open, onToggle }: BarProps) {
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
      <span className={`last-line level-${lastLine?.level ?? 'info'}`} aria-live="polite">
        {lastLine ? lastLine.text : 'Ready'}
      </span>
      <span className="version" title="Version">
        {version}
      </span>
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
