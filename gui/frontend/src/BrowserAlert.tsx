// Shown over the window while the browser waits for the user: a check or
// captcha may need solving in its window. It goes away by itself once the
// page renders or its load ends; Dismiss hides it sooner.

import { useEffect, useRef } from 'react';
import { AlertIcon } from './icons';

interface Props {
  message: string;
  onShow: () => void;
  onDismiss: () => void;
}

export function BrowserAlert({ message, onShow, onDismiss }: Props) {
  const show = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    show.current?.focus();
  }, []);

  return (
    <div className="browser-alert-backdrop">
      <section
        className="browser-alert"
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="browser-alert-title"
        aria-describedby="browser-alert-text"
        onKeyDown={(e) => {
          if (e.key === 'Escape') {
            onDismiss();
          }
        }}
      >
        <h2 id="browser-alert-title">
          <AlertIcon size={18} /> The browser needs you
        </h2>
        <p id="browser-alert-text">{message}</p>
        <div className="browser-alert-actions">
          <button type="button" className="btn ghost" onClick={onDismiss}>
            Dismiss
          </button>
          <button type="button" className="btn primary" ref={show} onClick={onShow}>
            Show the browser window
          </button>
        </div>
      </section>
    </div>
  );
}
