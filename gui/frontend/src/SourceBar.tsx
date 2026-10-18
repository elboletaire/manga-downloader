// The top of the window: what to fetch. The series URL, and the two filters
// that shape the chapter list the site returns (language, scanlator). They sit
// together because all three only take effect when the series is (re)loaded.

import { useEffect, useRef, useState, type Ref } from 'react';
import { AUTO_LOAD_DELAY_MS, shouldAutoLoad } from './autoLoad';
import { SelectField, TextField } from './fields';
import { sameScanlator, typedValue, filterControl, filtersIdentified, filterOptions, shownSelection, type FilterControl, type FilterKind } from './filters';
import { say } from './i18n';
import { useI18n } from './i18n/react';
import { shortcutLabel, isMac } from './settingsShortcut';
import { AlertIcon, GearIcon, LinkIcon, RefreshIcon, Spinner } from './icons';
import { Mascot, useExpression, Wordmark } from './Mascot';
import type { FilterChoice, Options, State } from './types';

/** What the clipboard's text means for the URL box: trimmed, or '' when there is nothing to paste */
export function pasteValue(clipboard: string): string {
  return clipboard.trim();
}

interface Props {
  state: State;
  onLoad: (url: string) => void;
  /** The clipboard's text, read by the Paste button */
  readClipboard: () => Promise<string>;
  /** The URL box's clear button: empties the field and forgets the loaded series */
  onClear: () => void;
  onOptions: (patch: Partial<Options>) => Promise<void>;
  /** A filter picked from a selector: applied at once, with the series' reload. Its refusal is shown by the caller. */
  onPick: (patch: Partial<Options>) => Promise<void>;
  onError: (message: string) => void;
  /** A refusal of the last Load, shown with the load error */
  loadRefusal: string;
  /** A refused option (from here or the side panel), shown wherever the tab is */
  optionError: string;
  /** Opens the settings dialog from the gear button */
  onOpenSettings: () => void;
  settingsOpen: boolean;
  /** The gear button, so the dialog can hand focus back to it when it closes */
  settingsRef?: Ref<HTMLButtonElement>;
}

export function SourceBar({
  state,
  onLoad,
  readClipboard,
  onClear,
  onOptions,
  onPick,
  onError,
  loadRefusal,
  optionError,
  onOpenSettings,
  settingsOpen,
  settingsRef,
}: Props) {
  const { t } = useI18n();
  // null until the user types: the field shows the backend's URL until then
  const [typed, setTyped] = useState<string | null>(null);
  const url = typed ?? state.url;
  const busy = state.loading || state.running;
  const expression = useExpression(state);
  const same = state.loaded && url.trim() === state.url;
  // a pasted or typed series URL loads once the typing pauses; the timer reads
  // the latest state, since it fires after the render that started it
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const latest = useRef({ url: state.url, busy });
  useEffect(() => {
    latest.current = { url: state.url, busy };
  }, [state.url, busy]);
  useEffect(() => () => clearTimeout(timer.current), []);
  // a pasted bare host may load on its own, a typed one waits for Load or Enter
  // (see onPaste and shouldAutoLoad)
  const pasting = useRef(false);
  const changeUrl = (value: string, pasted = false) => {
    setTyped(value);
    clearTimeout(timer.current);
    timer.current = setTimeout(() => {
      if (shouldAutoLoad(value, latest.current, { pasted })) {
        onLoad(value.trim());
      }
    }, AUTO_LOAD_DELAY_MS);
  };
  const input = useRef<HTMLInputElement>(null);
  // the pasted text goes through changeUrl like typed text, marked as pasted; the
  // field keeps the focus whether or not there was anything to paste
  const paste = () => {
    readClipboard()
      .then((text) => {
        const value = pasteValue(text);
        // text typed while the clipboard was read wins over the paste
        if (value !== '' && input.current?.value === '') {
          changeUrl(value, true);
        }
      })
      .catch(() => undefined)
      .finally(() => input.current?.focus());
  };
  // each filter is stale on its own: only the one whose value differs from the
  // loaded list is flagged
  // free text until the first load; after it, each filter is the series' own
  // selector, and a picked value reloads at once, so the filters are never
  // left unapplied
  const identified = filtersIdentified(state);
  const languageStale = state.loaded && state.options.language !== state.loadedLanguage;
  const scanlatorStale = state.loaded && !sameScanlator(state.options.scanlator, state.loadedScanlator);
  // a pick is applied with the reload of the series, in one step (see onPick)
  const pick = (patch: Partial<Options>) => {
    void onPick(patch);
  };
  // a failed load with nothing listed is told in the chapter area instead
  const problem = loadRefusal !== '' ? loadRefusal : state.loaded ? say(t, state.loadErrorMsg, state.loadError) : '';
  const settingsLabel = `${t('settings.open')} (${shortcutLabel(isMac())})`;

  return (
    <header className="source">
      <div className="source-row">
        <div className="brand" title="Manga Downloader">
          <Mascot expression={expression} size={48} />
          <Wordmark />
        </div>
        <form
          className="url-form"
          onSubmit={(e) => {
            e.preventDefault();
            // Enter loads now, so the pending automatic load must not fire too
            clearTimeout(timer.current);
            onLoad(url);
          }}
        >
          <div className={`url-box${problem !== '' ? ' invalid' : ''}`}>
            <LinkIcon className="url-icon" />
            <input
              ref={input}
              type="url"
              className="url-input"
              aria-label={t('source.url.label')}
              placeholder={t('source.url.placeholder')}
              value={url}
              // readOnly, not disabled: a disabled input drops focus while a load runs
              readOnly={busy}
              aria-busy={busy}
              spellCheck={false}
              autoComplete="off"
              onChange={(e) => {
                changeUrl(e.target.value, pasting.current);
                pasting.current = false;
              }}
              onPaste={() => {
                pasting.current = true;
                // a paste that changes nothing fires no change, so the mark is dropped after it
                setTimeout(() => {
                  pasting.current = false;
                });
              }}
              onFocus={(e) => e.currentTarget.select()}
            />
            {url !== '' && !busy && (
              <button
                type="button"
                className="url-clear"
                aria-label={t('source.url.clearLabel')}
                title={t('source.url.clear')}
                onClick={() => {
                  // the pending automatic load must not fire on the emptied field
                  clearTimeout(timer.current);
                  setTyped('');
                  onClear();
                }}
              >
                ×
              </button>
            )}
            {url === '' && !busy && (
              <button
                type="button"
                className="btn small ghost url-paste"
                aria-label={t('source.url.pasteLabel')}
                title={t('source.url.pasteLabel')}
                onClick={paste}
              >
                {t('source.url.paste')}
              </button>
            )}
          </div>
          <button
            type="submit"
            className={`btn load-btn${same ? '' : ' primary'}`}
            disabled={busy || url.trim() === ''}
          >
            {state.loading ? (
              <>
                <Spinner /> {t('source.load.loading')}
              </>
            ) : same ? (
              <>
                <RefreshIcon /> {t('source.load.reload')}
              </>
            ) : (
              t('source.load.load')
            )}
          </button>
        </form>
        <button
          ref={settingsRef}
          type="button"
          className="btn ghost icon-only settings-btn"
          aria-label={settingsLabel}
          title={settingsLabel}
          aria-haspopup="dialog"
          aria-expanded={settingsOpen}
          onClick={onOpenSettings}
        >
          <GearIcon />
        </button>
      </div>

      <div className="filter-row">
        <span className="filter-label">{t('source.filter.heading')}</span>
        <FilterField
          kind="language"
          label={t('source.filter.language')}
          typed={state.options.language}
          choice={state.languages}
          control={filterControl(state, state.languages)}
          loaded={state.loaded}
          busy={busy}
          flagged={languageStale}
          onPick={(language) => pick({ language })}
          onOptions={onOptions}
          onError={onError}
        />
        <FilterField
          kind="scanlator"
          label={t('source.filter.scanlator')}
          typed={state.options.scanlator}
          choice={state.scanlators}
          control={filterControl(state, state.scanlators)}
          loaded={state.loaded}
          busy={busy}
          flagged={scanlatorStale}
          onPick={(scanlator) => pick({ scanlator })}
          onOptions={onOptions}
          onError={onError}
        />
        {state.stale ? (
          <div className="stale-note" role="status">
            <AlertIcon />
            <span>{t('source.filter.notApplied')}</span>
            <button type="button" className="btn small warning" disabled={busy} onClick={() => onLoad(state.url)}>
              <RefreshIcon /> {t('source.load.reload')}
            </button>
          </div>
        ) : (
          <span className="filter-hint">
            {identified ? t('source.filter.appliesAtOnce') : t('source.filter.appliesOnLoad')}
          </span>
        )}
      </div>

      {problem !== '' && !state.loading && (
        <div className="banner danger" role="alert">
          <AlertIcon />
          <span>{problem}</span>
        </div>
      )}
      {optionError !== '' && (
        <div className="banner danger" role="alert">
          <AlertIcon />
          <span>{optionError}</span>
        </div>
      )}
    </header>
  );
}

interface FilterFieldProps {
  kind: FilterKind;
  label: string;
  /** The value typed (or last picked), shown until a load and by a site with no choice */
  typed: string;
  /** What the loaded series offers, null when the site lists none */
  choice: FilterChoice | null;
  control: FilterControl;
  /** Whether the series loaded: a failed load's typed value isn't an option */
  loaded: boolean;
  busy: boolean;
  flagged: boolean;
  onPick: (value: string) => void;
  onOptions: (patch: Partial<Options>) => Promise<void>;
  onError: (message: string) => void;
}

/** One filter: free text before a load, then a selector or a disabled field */
function FilterField({
  kind,
  label,
  typed,
  choice,
  control,
  loaded,
  busy,
  flagged,
  onPick,
  onOptions,
  onError,
}: FilterFieldProps) {
  const { t, lang } = useI18n();
  if (control === 'text') {
    return (
      <TextField
        className="inline-field"
        label={label}
        value={typedValue(typed)}
        placeholder={t('source.filter.any')}
        disabled={busy}
        flagged={flagged}
        onCommit={(value) => onOptions(kind === 'language' ? { language: value } : { scanlator: value })}
        onError={onError}
      />
    );
  }
  if (control === 'disabled' || choice === null) {
    // cleared: the site ignores the value, so showing it would suggest it
    // applies. The saved value stays, for the sites that do take it.
    return (
      <TextField
        className="inline-field"
        label={label}
        value=""
        placeholder={t('source.filter.any')}
        disabled
        hint={kind === 'language' ? t('source.filter.noLanguage') : t('source.filter.noScanlator')}
        onCommit={() => Promise.resolve()}
        onError={onError}
      />
    );
  }

  return (
    <SelectField
      className="inline-field"
      label={label}
      value={shownSelection(choice, loaded)}
      options={filterOptions(choice, kind, t, lang, loaded)}
      disabled={busy}
      onChange={onPick}
    />
  );
}
