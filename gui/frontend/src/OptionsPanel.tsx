import { useEffect, useState } from 'react';
import { convertFlagsOf, convertValueOf } from './convertImages';
import type { Options, State } from './types';

interface Props {
  options: Options;
  state: State;
  disabled: boolean;
  onCommit: (o: Options) => Promise<void>;
  onChooseFolder: () => Promise<void>;
  onReload: () => void;
  onError: (message: string) => void;
}

/**
 * The options form. Edits stay local until they're committed: checkboxes and
 * selects commit at once, text fields on blur or Enter. The form takes the
 * committed options back whenever they change, so it never shows a value the
 * Go side refused.
 */
export function OptionsPanel({ options, state, disabled, onCommit, onChooseFolder, onReload, onError }: Props) {
  const [draft, setDraft] = useState<Options>(options);
  const committed = JSON.stringify(options);

  useEffect(() => {
    setDraft(options);
    // keyed on the JSON, so a state push with the same options keeps the draft
    // the user is still typing in
  }, [committed]);

  const commit = (next: Options) => {
    setDraft(next);
    onCommit(next).catch((e: unknown) => {
      setDraft(options);
      onError(typeof e === 'string' ? e : e instanceof Error ? e.message : String(e));
    });
  };
  const commitText = () => {
    if (JSON.stringify(draft) !== committed) {
      commit(draft);
    }
  };
  const set = <K extends keyof Options>(key: K, value: Options[K]) => setDraft({ ...draft, [key]: value });

  const flags = convertFlagsOf(draft.convertImages);
  const locked = disabled || state.loading || state.running;

  return (
    <section className="panel options">
      <h2>Options</h2>

      <label className="field">
        <span>Output folder</span>
        <div className="inline">
          <input type="text" readOnly value={draft.outputDir} aria-label="Output folder" />
          <button type="button" disabled={locked} onClick={() => onChooseFolder().catch((e) => onError(String(e)))}>
            Browse…
          </button>
        </div>
      </label>

      <label className="field">
        <span>Format</span>
        <select
          value={draft.format}
          disabled={locked}
          onChange={(e) => commit({ ...draft, format: e.target.value as Options['format'] })}
        >
          <option value="cbz">CBZ (one archive per chapter)</option>
          <option value="raw">Raw (page images in a folder)</option>
        </select>
      </label>

      <label className="check">
        <input
          type="checkbox"
          checked={draft.bundle}
          disabled={locked}
          onChange={(e) => commit({ ...draft, bundle: e.target.checked })}
        />
        <span>Bundle all chapters into one file</span>
      </label>

      <fieldset className="field" disabled={locked}>
        <legend>Convert images</legend>
        <label className="check">
          <input
            type="checkbox"
            checked={flags.avif}
            onChange={(e) => commit({ ...draft, convertImages: convertValueOf({ ...flags, avif: e.target.checked }) })}
          />
          <span>AVIF → JPEG</span>
        </label>
        <label className="check">
          <input
            type="checkbox"
            checked={flags.webp}
            onChange={(e) => commit({ ...draft, convertImages: convertValueOf({ ...flags, webp: e.target.checked }) })}
          />
          <span>WebP → JPEG</span>
        </label>
      </fieldset>

      <label className="field">
        <span>Language</span>
        <input
          type="text"
          value={draft.language}
          disabled={locked}
          placeholder="e.g. en"
          onChange={(e) => set('language', e.target.value)}
          onBlur={commitText}
          onKeyDown={(e) => e.key === 'Enter' && commitText()}
        />
      </label>

      <label className="field">
        <span>Scanlator</span>
        <input
          type="text"
          value={draft.scanlator}
          disabled={locked}
          onChange={(e) => set('scanlator', e.target.value)}
          onBlur={commitText}
          onKeyDown={(e) => e.key === 'Enter' && commitText()}
        />
      </label>
      {state.loaded && (
        <div className="stale">
          {state.stale ? (
            <>
              <p>The language or scanlator changed. Reload the series to apply it to the chapter list.</p>
              <button type="button" disabled={locked} onClick={onReload}>
                Apply and reload
              </button>
            </>
          ) : (
            <p className="hint">Language and scanlator filter the chapter list, so changing them needs a reload.</p>
          )}
        </div>
      )}

      <label className="field">
        <span>Skip pages</span>
        <input
          type="text"
          value={draft.skipPages}
          disabled={locked}
          placeholder="e.g. 2,-1"
          onChange={(e) => set('skipPages', e.target.value)}
          onBlur={commitText}
          onKeyDown={(e) => e.key === 'Enter' && commitText()}
        />
      </label>

      <label className="field">
        <span>Filename template</span>
        <input
          type="text"
          value={draft.filenameTemplate}
          disabled={locked}
          onChange={(e) => set('filenameTemplate', e.target.value)}
          onBlur={commitText}
          onKeyDown={(e) => e.key === 'Enter' && commitText()}
        />
      </label>

      <label className="check">
        <input
          type="checkbox"
          checked={draft.browserVisible}
          disabled={locked}
          onChange={(e) => commit({ ...draft, browserVisible: e.target.checked })}
        />
        <span>Show the browser window (for sites that need one)</span>
      </label>
    </section>
  );
}
