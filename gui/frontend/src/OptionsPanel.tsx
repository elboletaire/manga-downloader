// How the chapters are saved. Every control commits on its own (see fields.tsx)
// as a patch; App applies each patch over the last options sent, so two
// controls can't overwrite each other's edits.

import { browserSwitchOf } from './browserVisible';
import { avifEpubHint, avifSwitchOf, convertFlagsOf, convertValueOf } from './convertImages';
import { Check, Segmented, Switch, TextField } from './fields';
import { errorMessage } from './format';
import type { T as Translator } from './i18n';
import { T, useI18n } from './i18n/react';
import { FolderIcon, RefreshIcon } from './icons';
import type { Options, PackAs, State } from './types';

export function packHints(t: Translator): Record<PackAs, string> {
  return {
    chapters: t('options.packHint.chapters'),
    parts: t('options.packHint.parts'),
    volumes: t('options.packHint.volumes'),
    bundle: t('options.packHint.bundle'),
  };
}

/** The template's tokens, passed as values so they stay as written */
const TEMPLATE_TOKENS = {
  series: '{{.Series}}',
  number: '{{.Number}}',
  title: '{{.Title}}',
  siteTitle: '{{.SiteTitle}}',
  group: '{{.Group}}',
  volume: '{{.Volume}}',
  version: '{{.Version}}',
  padded: '{{pad .Number 4}}',
  volumeName: 'Series Vol. 01 (1-7)',
  volumeGroupName: 'Series Vol. 01 (1-7) [Group]',
};

interface Props {
  state: State;
  onOptions: (patch: Partial<Options>) => Promise<void>;
  onChooseFolder: () => Promise<void>;
  onError: (message: string) => void;
}

export function OptionsPanel({ state, onOptions, onChooseFolder, onError }: Props) {
  const { t } = useI18n();
  const o = state.options;
  const locked = state.loading || state.running;
  const flags = convertFlagsOf(o.convertImages);
  const avif = avifSwitchOf(flags, o.format);
  const browser = browserSwitchOf(state, t);
  const hints = packHints(t);
  const set = (patch: Partial<Options>) => {
    onOptions(patch).catch(() => undefined);
  };

  return (
    <div className="options">
      {locked && (
        <p className="locked-note" role="status">
          {state.running ? t('options.locked.running') : t('options.locked.loading')}
        </p>
      )}
      <section className="opt-group" aria-labelledby="opt-save">
        <h3 id="opt-save">{t('options.save.title')}</h3>
        <div className="field">
          <span className="label" id="folder-label">
            {t('options.save.folder')}
          </span>
          <div className="folder">
            <span className="folder-path mono" title={o.outputDir} aria-labelledby="folder-label">
              <FolderIcon size={14} />
              {/* bdi + rtl keeps the end of a long path, the part that tells folders apart */}
              <span className="folder-text">
                <bdi>{o.outputDir || t('options.save.notSet')}</bdi>
              </span>
            </span>
            <button
              type="button"
              className="btn small"
              disabled={locked}
              onClick={() => onChooseFolder().catch((e) => onError(errorMessage(e, t)))}
            >
              {t('options.save.browse')}
            </button>
          </div>
        </div>

        <Segmented<Options['format']>
          label={t('options.format.label')}
          value={o.format}
          disabled={locked}
          options={[
            { value: 'cbz', label: t('options.format.cbz') },
            { value: 'cbr', label: t('options.format.cbr') },
            { value: 'raw', label: t('options.format.raw') },
            { value: 'epub', label: t('options.format.epub') },
          ]}
          onChange={(format) => set({ format })}
        />

        {o.format === 'epub' && (
          <div className="field-stack">
            <Segmented<Options['readingDirection']>
              label={t('options.readingDirection.label')}
              value={o.readingDirection}
              disabled={locked}
              options={[
                { value: 'rtl', label: t('options.readingDirection.rtl') },
                { value: 'ltr', label: t('options.readingDirection.ltr') },
              ]}
              onChange={(readingDirection) => set({ readingDirection })}
            />
            <p className="hint">{t('options.readingDirection.hint')}</p>
          </div>
        )}

        <div className="field-stack">
          <Segmented<PackAs>
            label={t('options.packAs.label')}
            value={o.packAs}
            disabled={locked}
            options={[
              { value: 'chapters', label: t('options.packAs.chapters') },
              { value: 'parts', label: t('options.packAs.parts') },
              { value: 'volumes', label: t('options.packAs.volumes') },
              { value: 'bundle', label: t('options.packAs.bundle') },
            ]}
            onChange={(packAs) => set({ packAs })}
          />
          <p className="hint">
            {o.packAs === 'volumes' && !state.loaded ? t('options.packHint.volumesNotLoaded') : hints[o.packAs]}
          </p>
        </div>
      </section>

      <section className="opt-group" aria-labelledby="opt-pages">
        <h3 id="opt-pages">{t('options.pages.title')}</h3>
        <fieldset className="field" disabled={locked}>
          <legend>{t('options.pages.convert')}</legend>
          <div className="check-row">
            <Check
              label={t('options.pages.avif')}
              checked={avif.checked}
              disabled={avif.disabled}
              onChange={(checked) => set({ convertImages: convertValueOf({ ...flags, avif: checked }) })}
            />
            <Check
              label={t('options.pages.webp')}
              checked={flags.webp}
              onChange={(webp) => set({ convertImages: convertValueOf({ ...flags, webp }) })}
            />
          </div>
          <p className="hint">{o.format === 'epub' ? avifEpubHint(t) : t('options.pages.convertHint')}</p>
        </fieldset>

        <Switch
          label={t('options.pages.compress.label')}
          description={t('options.pages.compress.hint')}
          checked={o.compress}
          disabled={locked}
          onChange={(compress) => set({ compress })}
        />

        <TextField
          label={t('options.skipPages.label')}
          value={o.skipPages}
          placeholder={t('options.skipPages.placeholder', { example: '2,-1' })}
          mono
          disabled={locked}
          hint={t('options.skipPages.hint', { first: '2', last: '-1' })}
          onCommit={(skipPages) => onOptions({ skipPages })}
          onError={onError}
        />
      </section>

      <details className="opt-group advanced">
        <summary>
          <h3>{t('options.advanced.title')}</h3>
        </summary>
        <div className="opt-group-body">
          <TextField
            label={t('options.template.label')}
            value={o.filenameTemplate}
            placeholder={state.filenameTemplateDefault}
            mono
            disabled={locked}
            hint={<T k="options.template.hint" values={TEMPLATE_TOKENS} />}
            action={
              o.filenameTemplate !== state.filenameTemplateDefault && (
                <button
                  type="button"
                  className="btn ghost small icon-only"
                  aria-label={t('options.template.reset')}
                  title={t('options.template.reset')}
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
          <TextField
            label={t('options.pages.quality.label')}
            value={String(o.jpegQuality)}
            mono
            disabled={locked}
            hint={t('options.pages.quality.hint')}
            onCommit={(text) => {
              const quality = Number(text);
              if (!Number.isInteger(quality) || quality < 1 || quality > 100) {
                return Promise.reject(new Error(t('options.pages.quality.invalid')));
              }
              return onOptions({ jpegQuality: quality });
            }}
            onError={onError}
          />
          <Switch
            label={t('options.browser.label')}
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
