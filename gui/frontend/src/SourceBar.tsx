// The top of the window: what to fetch. The series URL, and the two filters
// that shape the chapter list the site returns (language, scanlator). They sit
// together because all three only take effect when the series is (re)loaded.

import { useEffect, useRef, useState } from 'react';
import { AUTO_LOAD_DELAY_MS, shouldAutoLoad } from './autoLoad';
import { SelectField, TextField } from './fields';
import { filterControl, filtersIdentified, filterOptions, type FilterControl, type FilterKind } from './filters';
import { AlertIcon, LinkIcon, RefreshIcon, Spinner } from './icons';
import { Mascot, useExpression } from './Mascot';
import type { FilterChoice, Options, State } from './types';

interface Props {
  state: State;
  onLoad: (url: string) => void;
  onOptions: (patch: Partial<Options>) => Promise<void>;
  onError: (message: string) => void;
  /** A refusal of the last Load, shown with the load error */
  loadRefusal: string;
  /** A refused option (from here or the side panel), shown wherever the tab is */
  optionError: string;
}

export function SourceBar({ state, onLoad, onOptions, onError, loadRefusal, optionError }: Props) {
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
  const changeUrl = (value: string) => {
    setTyped(value);
    clearTimeout(timer.current);
    timer.current = setTimeout(() => {
      if (shouldAutoLoad(value, latest.current)) {
        onLoad(value.trim());
      }
    }, AUTO_LOAD_DELAY_MS);
  };
  // each filter is stale on its own: only the one whose value differs from the
  // loaded list is flagged
  // free text until the first load; after it, each filter is the series' own
  // selector, and a picked value reloads at once, so the filters are never
  // left unapplied
  const identified = filtersIdentified(state);
  const languageStale = state.loaded && state.options.language !== state.loadedLanguage;
  const scanlatorStale = state.loaded && state.options.scanlator !== state.loadedScanlator;
  const pick = (patch: Partial<Options>) => {
    onOptions(patch)
      .then(() => onLoad(state.url))
      // the refusal is already shown by onOptions' caller
      .catch(() => undefined);
  };
  // a failed load with nothing listed is told in the chapter area instead
  const problem = loadRefusal !== '' ? loadRefusal : state.loaded ? state.loadError : '';

  return (
    <header className="source">
      <div className="source-row">
        <div className="brand" title="Manga Downloader">
          <Mascot expression={expression} size={48} />
          <span className="brand-name">Manga Downloader</span>
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
              type="url"
              className="url-input"
              aria-label="Series URL"
              placeholder="Paste the link to a series or one of its chapters"
              value={url}
              // readOnly, not disabled: a disabled input drops focus while a load runs
              readOnly={busy}
              aria-busy={busy}
              spellCheck={false}
              autoComplete="off"
              onChange={(e) => changeUrl(e.target.value)}
              onFocus={(e) => e.currentTarget.select()}
            />
            {url !== '' && !busy && (
              <button
                type="button"
                className="url-clear"
                aria-label="Clear the URL"
                title="Clear"
                onClick={() => changeUrl('')}
              >
                ×
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
                <Spinner /> Loading…
              </>
            ) : same ? (
              <>
                <RefreshIcon /> Reload
              </>
            ) : (
              'Load'
            )}
          </button>
        </form>
      </div>

      <div className="filter-row">
        <span className="filter-label">Filter the list</span>
        <FilterField
          kind="language"
          label="Language"
          typed={state.options.language}
          choice={state.languages}
          control={filterControl(state, state.languages)}
          busy={busy}
          flagged={languageStale}
          onPick={(language) => pick({ language })}
          onOptions={onOptions}
          onError={onError}
        />
        <FilterField
          kind="scanlator"
          label="Scanlator"
          typed={state.options.scanlator}
          choice={state.scanlators}
          control={filterControl(state, state.scanlators)}
          busy={busy}
          flagged={scanlatorStale}
          onPick={(scanlator) => pick({ scanlator })}
          onOptions={onOptions}
          onError={onError}
        />
        {state.stale ? (
          <div className="stale-note" role="status">
            <AlertIcon />
            <span>Not applied yet</span>
            <button type="button" className="btn small warning" disabled={busy} onClick={() => onLoad(state.url)}>
              <RefreshIcon /> Reload
            </button>
          </div>
        ) : (
          <span className="filter-hint">
            {identified ? 'Picking one reloads the series' : 'Applied when the series loads'}
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
  busy,
  flagged,
  onPick,
  onOptions,
  onError,
}: FilterFieldProps) {
  if (control === 'text') {
    return (
      <TextField
        className="inline-field"
        label={label}
        value={typed}
        placeholder="any"
        disabled={busy}
        flagged={flagged}
        onCommit={(value) => onOptions(kind === 'language' ? { language: value } : { scanlator: value })}
        onError={onError}
      />
    );
  }
  if (control === 'disabled' || choice === null) {
    return (
      <TextField
        className="inline-field"
        label={label}
        value={typed}
        placeholder="any"
        disabled
        hint={kind === 'language' ? 'This site has no language choice' : 'This site has no scanlation groups to choose from'}
        onCommit={() => Promise.resolve()}
        onError={onError}
      />
    );
  }

  return (
    <SelectField
      className="inline-field"
      label={label}
      value={choice.selected}
      options={filterOptions(choice, kind)}
      disabled={busy}
      onChange={onPick}
    />
  );
}
