// The settings dialog: the app-only preferences (language, update check) and
// the About footer. Controls apply at once, so there is no Save or Cancel.

import { useEffect, useRef, type MouseEvent } from 'react';
import { errorMessage } from './format';
import { LANG_INFO, LANGS, type Lang, type LangPref } from './i18n';
import { T, useI18n } from './i18n/react';
import { SelectField, Switch } from './fields';
import { XIcon } from './icons';
import type { State } from './types';

interface Props {
  state: State;
  onClose: () => void;
  onCheckUpdates: (on: boolean) => void;
  onOpenURL: (url: string) => void;
  onError: (msg: string) => void;
}

const ISSUES_URL = 'https://github.com/elboletaire/manga-downloader/issues';
const LICENSE = 'AGPL-3.0';
export const LICENSE_URL = 'https://github.com/elboletaire/manga-downloader/blob/master/LICENSE';
const AUTHOR = 'elboletaire';
const AUTHOR_URL = 'https://github.com/elboletaire';
// the years of the source files' copyright headers
const YEARS = '2023–2026';

const WEBSITE_PATHS: Record<Lang, string> = { en: '', es: 'es/', ca: 'ca/' };

/** The project's website in a language: English at the root, the others under their own path */
export function websiteURL(lang: Lang): string {
  return `https://mangadownloader.cat/${WEBSITE_PATHS[lang]}`;
}

export function SettingsDialog({ state, onClose, onCheckUpdates, onOpenURL, onError }: Props) {
  const { t, pref, systemLang, lang, setLanguage } = useI18n();
  const dialog = useRef<HTMLDialogElement>(null);
  const devBuild = state.releaseURL === '';

  useEffect(() => {
    const el = dialog.current;
    if (el && !el.open) el.showModal();
    // closing on unmount hands focus back to the element that opened the dialog
    return () => {
      if (el?.open) el.close();
    };
  }, []);

  const pick = (next: LangPref) => {
    setLanguage(next).catch((e: unknown) => onError(errorMessage(e, t)));
  };

  // A click on the dialog itself is a backdrop click, but only when the press started there too.
  const pressedBackdrop = useRef(false);
  const onBackdropDown = (e: MouseEvent<HTMLDialogElement>) => {
    pressedBackdrop.current = e.target === e.currentTarget;
  };
  const onBackdrop = (e: MouseEvent<HTMLDialogElement>) => {
    if (pressedBackdrop.current && e.target === e.currentTarget) onClose();
  };

  const link = (url: string) => (e: MouseEvent<HTMLAnchorElement>) => {
    e.preventDefault();
    onOpenURL(url);
  };

  // the endonyms stay untranslated; System names the language it resolves to
  const languageOptions = [
    { value: '', label: t('settings.language.systemOption', { language: LANG_INFO[systemLang].name }) },
    ...LANGS.map((l) => ({ value: l, label: LANG_INFO[l].name })),
  ];

  return (
    <dialog
      ref={dialog}
      className="settings-dialog"
      aria-labelledby="settings-title"
      onCancel={(e) => {
        e.preventDefault();
        onClose();
      }}
      onMouseDown={onBackdropDown}
      onClick={onBackdrop}
    >
      <div className="settings-content">
        <header className="settings-head">
          <h2 id="settings-title">{t('settings.title')}</h2>
          <button type="button" className="btn ghost icon-only" aria-label={t('settings.close')} onClick={onClose}>
            <XIcon size={16} />
          </button>
        </header>

        <div className="settings-body">
          <section className="opt-group">
            <h3>{t('settings.language.heading')}</h3>
            <SelectField
              label={t('settings.language.heading')}
              value={pref}
              options={languageOptions}
              onChange={(value) => pick(value as LangPref)}
            />
          </section>

          <section className="opt-group">
            <h3>{t('settings.updates.heading')}</h3>
            <Switch
              label={t('settings.updates.check')}
              description={devBuild ? t('settings.updates.devHint') : t('settings.updates.hint')}
              checked={!devBuild && state.prefs.checkUpdates}
              disabled={devBuild}
              onChange={onCheckUpdates}
            />
          </section>
        </div>

        <footer className="settings-about">
          {state.releaseURL !== '' ? (
            <a href={state.releaseURL} onClick={link(state.releaseURL)}>
              {t('settings.about.version', { version: state.version })}
            </a>
          ) : (
            <span>{t('settings.about.version', { version: state.version })}</span>
          )}
          <a href={websiteURL(lang)} onClick={link(websiteURL(lang))}>
            {t('settings.about.website')}
          </a>
          <a href={ISSUES_URL} onClick={link(ISSUES_URL)}>
            {t('settings.about.issues')}
          </a>
          <p className="settings-author">
            <T
              k="settings.about.madeBy"
              values={{ author: AUTHOR, years: YEARS, license: LICENSE }}
              components={{
                author: (children) => (
                  <a href={AUTHOR_URL} onClick={link(AUTHOR_URL)}>
                    {children}
                  </a>
                ),
                license: (children) => (
                  <a href={LICENSE_URL} onClick={link(LICENSE_URL)}>
                    {children}
                  </a>
                ),
              }}
            />
          </p>
        </footer>
      </div>
    </dialog>
  );
}
