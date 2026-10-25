// The log lives in the status bar: its newest line is always in view, and the
// whole of it opens as a drawer over the window. Warnings and errors are
// counted on the toggle, so a problem isn't missed with the drawer closed.

import { useEffect, useRef, useState, type MouseEvent } from "react";
import { useI18n } from "./i18n/react";
import {
  AlertIcon,
  CopyIcon,
  ErrorIcon,
  KofiIcon,
  LogIcon,
  XIcon,
} from "./icons";
import { KOFI_URL, linkClick } from "./links";
import { UpdateDialog } from "./UpdateDialog";
import { say } from "./i18n";
import type { LogLine, Update } from "./types";

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

// the update's link: a one-click update opens its dialog, any other opens the release
export function updateClick(
  update: Update,
  openDialog: () => void,
  openURL: (url: string) => void,
) {
  return (e: MouseEvent<HTMLAnchorElement>) => {
    e.preventDefault();
    if (update.oneClick) {
      openDialog();
    } else {
      openURL(update.url);
    }
  };
}

export function StatusBar({
  log,
  version,
  releaseURL,
  update,
  open,
  onToggle,
  onOpenURL,
  onUpdate,
}: BarProps) {
  const { t } = useI18n();
  const [updateOpen, setUpdateOpen] = useState(update?.prompt === true);

  const openLink = (url: string) => linkClick(url, onOpenURL);

  const onUpdateClick = update
    ? updateClick(update, () => setUpdateOpen(true), onOpenURL)
    : undefined;

  const lastLine = log.length > 0 ? log[log.length - 1] : undefined;
  let warns = 0;
  let errors = 0;
  for (const l of log) {
    if (l.level === "warn") {
      warns++;
    } else if (l.level === "error") {
      errors++;
    }
  }

  return (
    <footer className="statusbar">
      <button
        type="button"
        className={`log-toggle${open ? " open" : ""}`}
        aria-expanded={open}
        aria-controls="log-drawer"
        onClick={onToggle}
        title={open ? t("log.hide") : t("log.show")}
      >
        <LogIcon size={14} />
        <span>{t("log.title")}</span>
        {errors > 0 && (
          <span
            className="count-badge danger"
            aria-label={t("count.error", { count: errors })}
          >
            {errors}
          </span>
        )}
        {warns > 0 && (
          <span
            className="count-badge warning"
            aria-label={t("count.warning", { count: warns })}
          >
            {warns}
          </span>
        )}
      </button>
      <span
        className={`last-line level-${lastLine?.level ?? "info"}`}
        aria-live="polite"
        title={lastLine && say(t, lastLine.msg, lastLine.text)}
      >
        {lastLine ? say(t, lastLine.msg, lastLine.text) : t("log.ready")}
      </span>
      <span className="version-group">
        {update && (
          <a
            className="update-badge warning"
            href={update.url}
            onClick={onUpdateClick}
            title={t("log.updateAvailable", { latest: update.latest })}
            aria-label={t("log.updateAvailable", { latest: update.latest })}
          >
            <svg
              viewBox="0 0 16 16"
              width="16"
              height="16"
              aria-hidden="true"
              focusable="false"
            >
              <circle
                className="update-disc"
                cx="8"
                cy="8"
                r="7"
                strokeWidth="1.5"
              />
              <line
                x1="8"
                y1="4.6"
                x2="8"
                y2="8.6"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
              />
              <circle
                cx="8"
                cy="11.2"
                r="1.1"
                fill="currentColor"
                stroke="none"
              />
            </svg>
          </a>
        )}
        {update ? (
          <a
            className="version"
            href={update.url}
            onClick={onUpdateClick}
            title={t("log.updateAvailable", { latest: update.latest })}
          >
            {version}
          </a>
        ) : releaseURL !== "" ? (
          <a
            className="version"
            href={releaseURL}
            onClick={openLink(releaseURL)}
            title={t("log.versionRelease")}
          >
            {version}
          </a>
        ) : (
          <span className="version" title={t("log.version")}>
            {version}
          </span>
        )}
        <a
          className="kofi"
          href={KOFI_URL}
          onClick={openLink(KOFI_URL)}
          title={t("log.supportKofi")}
          aria-label={t("log.supportKofi")}
        >
          <KofiIcon size={18} />
        </a>
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
  const { t } = useI18n();
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
    const text = log
      .map((l) => {
        const shown = say(t, l.msg, l.text);
        return l.level === "info"
          ? shown
          : `${l.level.toUpperCase()}: ${shown}`;
      })
      .join("\n");
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
      aria-label={t("log.title")}
      onKeyDown={(e) => {
        if (e.key === "Escape") {
          onClose();
        }
      }}
    >
      <div className="log-head">
        <h2>{t("log.title")}</h2>
        <button
          type="button"
          className="btn ghost small"
          onClick={copy}
          disabled={log.length === 0}
        >
          <CopyIcon size={14} />{" "}
          {copied ? t("common.copied") : t("common.copy")}
        </button>
        <button
          type="button"
          className="btn ghost small icon-only"
          aria-label={t("log.closeLog")}
          onClick={onClose}
        >
          <XIcon size={14} />
        </button>
      </div>
      <div className="log-lines" ref={box} role="log" tabIndex={0}>
        {log.length === 0 && <p className="muted">{t("log.empty")}</p>}
        {log.map((line, i) => (
          <p key={i} className={`line level-${line.level}`}>
            {line.level === "error" ? (
              <ErrorIcon size={13} />
            ) : line.level === "warn" ? (
              <AlertIcon size={13} />
            ) : null}
            <span>{say(t, line.msg, line.text)}</span>
          </p>
        ))}
      </div>
    </section>
  );
}
