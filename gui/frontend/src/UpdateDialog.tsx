// The prompt behind the status bar's "!" when this copy can update itself.
// It reuses the browser alert's look; the backend logs a failure too, and it
// is shown here so the user doesn't have to open the log.

import { useEffect, useRef, useState } from 'react';
import { AlertIcon } from './icons';

interface Props {
  latest: string;
  current: string;
  onUpdate: () => Promise<void>;
  onReleaseNotes: () => void;
  onClose: () => void;
}

export function UpdateDialog({ latest, current, onUpdate, onReleaseNotes, onClose }: Props) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const update = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    update.current?.focus();
  }, []);

  const close = () => {
    if (!busy) {
      onClose();
    }
  };
  const run = () => {
    setBusy(true);
    setError('');
    onUpdate().catch((e: unknown) => {
      setError(String(e instanceof Error ? e.message : e));
      setBusy(false);
    });
  };

  return (
    <div
      className="browser-alert-backdrop"
      onClick={(e) => {
        if (e.target === e.currentTarget) {
          close();
        }
      }}
    >
      <section
        className="browser-alert"
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="update-dialog-title"
        aria-describedby="update-dialog-text"
        onKeyDown={(e) => {
          if (e.key === 'Escape') {
            close();
          }
        }}
      >
        <h2 id="update-dialog-title">
          <AlertIcon size={18} /> Update available
        </h2>
        <p id="update-dialog-text">
          Manga Downloader {latest} is available (you have {current}).
        </p>
        {error !== '' && (
          <p className="update-error" role="alert">
            {error}
          </p>
        )}
        <div className="browser-alert-actions">
          <button type="button" className="btn ghost" onClick={close} disabled={busy}>
            {error !== '' ? 'Close' : 'Later'}
          </button>
          <button type="button" className="btn ghost" onClick={onReleaseNotes} disabled={busy}>
            Release notes
          </button>
          <button type="button" className="btn primary" ref={update} onClick={run} disabled={busy}>
            {busy ? 'Downloading…' : 'Update now'}
          </button>
        </div>
      </section>
    </div>
  );
}
