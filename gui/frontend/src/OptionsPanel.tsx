// How the chapters are saved. Every control commits on its own (see fields.tsx)
// as a patch; App applies each patch over the last options sent, so two
// controls can't overwrite each other's edits.

import { browserSwitchOf } from './browserVisible';
import { AVIF_EPUB_HINT, avifSwitchOf, convertFlagsOf, convertValueOf } from './convertImages';
import { Check, Segmented, Switch, TextField } from './fields';
import { errorMessage } from './format';
import { FolderIcon, RefreshIcon } from './icons';
import type { Options, PackAs, State } from './types';

export const PACK_HINTS: Record<PackAs, string> = {
  chapters: 'One file per chapter.',
  parts: 'One file per chapter, with its parts merged into it. Chapters without parts get a file each as usual.',
  volumes: 'One file per volume, set above the chapter list. Chapters outside every volume get a file each.',
  bundle: 'The selected chapters in a single file.',
};

/** The volumes hint when no series is loaded: the box isn't visible yet */
export const VOLUMES_HINT_NOT_LOADED =
  'One file per volume, set above the chapter list (when loaded). Chapters outside every volume get a file each.';

interface Props {
  state: State;
  onOptions: (patch: Partial<Options>) => Promise<void>;
  onChooseFolder: () => Promise<void>;
  onError: (message: string) => void;
}

export function OptionsPanel({ state, onOptions, onChooseFolder, onError }: Props) {
  const o = state.options;
  const locked = state.loading || state.running;
  const flags = convertFlagsOf(o.convertImages);
  const avif = avifSwitchOf(flags, o.format);
  const browser = browserSwitchOf(state);
  const set = (patch: Partial<Options>) => {
    onOptions(patch).catch(() => undefined);
  };

  return (
    <div className="options">
      {locked && (
        <p className="locked-note" role="status">
          {state.running ? 'Options are locked while a download runs.' : 'Options are locked while the series loads.'}
        </p>
      )}
      <section className="opt-group" aria-labelledby="opt-save">
        <h3 id="opt-save">Save</h3>
        <div className="field">
          <span className="label" id="folder-label">
            Folder
          </span>
          <div className="folder">
            <span className="folder-path mono" title={o.outputDir} aria-labelledby="folder-label">
              <FolderIcon size={14} />
              {/* bdi + rtl keeps the end of a long path, the part that tells folders apart */}
              <span className="folder-text">
                <bdi>{o.outputDir || 'Not set'}</bdi>
              </span>
            </span>
            <button
              type="button"
              className="btn small"
              disabled={locked}
              onClick={() => onChooseFolder().catch((e) => onError(errorMessage(e)))}
            >
              Browse…
            </button>
          </div>
        </div>

        <Segmented<Options['format']>
          label="Format"
          value={o.format}
          disabled={locked}
          options={[
            { value: 'cbz', label: 'CBZ' },
            { value: 'cbr', label: 'CBR' },
            { value: 'raw', label: 'Folder' },
            { value: 'epub', label: 'EPUB' },
          ]}
          onChange={(format) => set({ format })}
        />

        {o.format === 'epub' && (
          <div className="field-stack">
            <Segmented<Options['readingDirection']>
              label="Reading direction"
              value={o.readingDirection}
              disabled={locked}
              options={[
                { value: 'rtl', label: 'Right to left' },
                { value: 'ltr', label: 'Left to right' },
              ]}
              onChange={(readingDirection) => set({ readingDirection })}
            />
            <p className="hint">Which way the pages turn: manga reads right to left, webtoons and western comics left to right.</p>
          </div>
        )}

        <div className="field-stack">
          <Segmented<PackAs>
            label="Pack as"
            value={o.packAs}
            disabled={locked}
            options={[
              { value: 'chapters', label: 'Chapters' },
              { value: 'parts', label: 'Merged' },
              { value: 'volumes', label: 'Volumes' },
              { value: 'bundle', label: 'One file' },
            ]}
            onChange={(packAs) => set({ packAs })}
          />
          <p className="hint">
            {o.packAs === 'volumes' && !state.loaded ? VOLUMES_HINT_NOT_LOADED : PACK_HINTS[o.packAs]}
          </p>
        </div>
      </section>

      <section className="opt-group" aria-labelledby="opt-pages">
        <h3 id="opt-pages">Pages</h3>
        <fieldset className="field" disabled={locked}>
          <legend>Convert to JPEG</legend>
          <div className="check-row">
            <Check
              label="AVIF"
              checked={avif.checked}
              disabled={avif.disabled}
              onChange={(checked) => set({ convertImages: convertValueOf({ ...flags, avif: checked }) })}
            />
            <Check
              label="WebP"
              checked={flags.webp}
              onChange={(webp) => set({ convertImages: convertValueOf({ ...flags, webp }) })}
            />
          </div>
          <p className="hint">
            {o.format === 'epub' ? AVIF_EPUB_HINT : "Most e-readers can't open AVIF; WebP is unreliable on e-ink."}
          </p>
        </fieldset>

        <TextField
          label="Skip pages"
          value={o.skipPages}
          placeholder="e.g. 2,-1"
          mono
          disabled={locked}
          hint="Positions dropped from every chapter: 2 is the second page, -1 the last."
          onCommit={(skipPages) => onOptions({ skipPages })}
          onError={onError}
        />
      </section>

      <details className="opt-group advanced">
        <summary>
          <h3>Advanced</h3>
        </summary>
        <div className="opt-group-body">
          <TextField
            label="Filename template"
            value={o.filenameTemplate}
            placeholder={state.filenameTemplateDefault}
            mono
            disabled={locked}
            hint={
              <>
                Uses <code>{'{{.Series}}'}</code>, <code>{'{{.Number}}'}</code>, <code>{'{{.Title}}'}</code>,{' '}
                <code>{'{{.SiteTitle}}'}</code>, <code>{'{{.Group}}'}</code>, <code>{'{{.Volume}}'}</code> and <code>{'{{.Version}}'}</code>;{' '}
                <code>{'{{pad .Number 4}}'}</code> zero-pads a number. Left as is, volumes are named{' '}
                <code>Series Vol. 01</code>, or <code>Series Vol. 01 [Group]</code> when their chapters share a group.
              </>
            }
            action={
              o.filenameTemplate !== state.filenameTemplateDefault && (
                <button
                  type="button"
                  className="btn ghost small icon-only"
                  aria-label="Use default template"
                  title="Use default template"
                  disabled={locked}
                  onClick={() => set({ filenameTemplate: state.filenameTemplateDefault })}
                >
                  <RefreshIcon />
                </button>
              )
            }
            onCommit={(filenameTemplate) => onOptions({ filenameTemplate })}
            onError={onError}
          />
          {state.filenameTemplateWarning && (
            <p className="hint warn-text" role="status">
              {state.filenameTemplateWarning}
            </p>
          )}
          <Switch
            label="Show the browser window"
            description={browser.hint}
            checked={browser.checked}
            disabled={browser.disabled}
            onChange={(browserVisible) => set({ browserVisible })}
          />
        </div>
      </details>
    </div>
  );
}
