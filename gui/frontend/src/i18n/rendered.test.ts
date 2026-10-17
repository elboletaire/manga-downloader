import { createElement, type ReactElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import type { Api } from '../api';
import { ActivityPanel } from '../ActivityPanel';
import { App } from '../App';
import { BrowserAlert } from '../BrowserAlert';
import { ChapterPanel } from '../ChapterPanel';
import { DownloadDock } from '../DownloadDock';
import { Check, SelectField, Segmented, Switch, TextField } from '../fields';
import { LogDrawer, StatusBar } from '../LogPanel';
import { createMockApi, type MockSetup } from '../mock';
import { OptionsPanel } from '../OptionsPanel';
import { SettingsDialog } from '../SettingsDialog';
import { SourceBar } from '../SourceBar';
import type { Chapter, LogLine, State } from '../types';
import { UpdateDialog } from '../UpdateDialog';
import { CATALOG, createT, LANG_INFO, say, type Catalog } from './index';
import { I18nProvider } from './react';

// The English catalog with every value wrapped in ⟦ ⟧: whatever shows
// unwrapped in a render is English that never went through t(). Placeholders
// and tags stay as they are.
const pseudo: Catalog = {
  en: Object.fromEntries(Object.entries(CATALOG.en ?? {}).map(([key, value]) => [key, `⟦${value}⟧`])),
};
const t = createT('en', pseudo);

// The English values a window shows: placeholders and tags stripped, 8 characters or more.
const plain = (value: string) =>
  value
    .replace(/\{\{[^}]*\}\}/g, ' ')
    .replace(/<\/?[a-z][a-z0-9]*>/gi, ' ')
    .replace(/\s+/g, ' ')
    .trim();
const escapeRegExp = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
const englishLeaks = Object.values(CATALOG.en ?? {})
  .map(plain)
  .filter((text) => text.length >= 8);

// The text the reader sees: tags dropped, except the attributes a reader hears
// or reads as text (labels, titles, placeholders, alt), with what came through
// t() removed. The brand is never translated, so it isn't English that skipped t().
const BRAND = 'Manga Downloader';
const untranslated = (html: string) => {
  const attributes = [...html.matchAll(/\s(?:aria-label|title|placeholder|alt)="([^"]*)"/g)].map((m) => m[1]);
  let text = [html.replace(/<[^>]*>/g, ' '), ...attributes]
    .join(' ')
    .replace(/&amp;/g, '&')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&#x27;/g, "'")
    .replaceAll(BRAND, ' ');
  // a value may hold other translated values, so the innermost wrappers go first
  for (let next = text.replace(/⟦[^⟧⟦]*⟧/g, ' '); next !== text; next = text.replace(/⟦[^⟧⟦]*⟧/g, ' ')) {
    text = next;
  }
  return text.replace(/\s+/g, ' ');
};

const SCREENS: Array<[string, Partial<MockSetup>]> = [
  ['empty', { scenario: 'empty' }],
  ['loading', { scenario: 'loading' }],
  ['error', { scenario: 'error' }],
  ['loaded', { scenario: 'loaded' }],
  ['volumes', { scenario: 'volumes' }],
  ['parts', { scenario: 'parts' }],
  ['running', { scenario: 'running' }],
  ['done with a failure', { scenario: 'done', fail: true }],
  ['update', { scenario: 'update' }],
  ['check', { scenario: 'check' }],
  ['bundle', { scenario: 'bundle' }],
  ['cancelling', { scenario: 'cancelling' }],
  ['stale', { scenario: 'stale' }],
  ['volumes running', { scenario: 'volumes-running' }],
  ['a site with no language or group choice', { scenario: 'loaded', site: 'none' }],
  ['a site with groups, and duplicates', { scenario: 'loaded', site: 'groups', dupes: true }],
  ['a site with its own languages', { scenario: 'loaded', site: 'plus' }],
  ['locked chapters, and no volumes', { scenario: 'loaded', locked: true, novolumes: true }],
  ['a run with skipped chapters, fetching their pages', { scenario: 'running', skip: true, fetching: true }],
  ['a finished EPUB run that failed', { scenario: 'done', fail: true, epub: true }],
];

interface Snapshot {
  state: State;
  chapters: Chapter[];
  selected: number[];
  log: LogLine[];
}

async function snapshot(setup: Partial<MockSetup>): Promise<Snapshot> {
  const api = createMockApi(setup);
  const [state, chapters, selected, log] = await Promise.all([
    api.state(),
    api.chapters(),
    api.selection(),
    api.log(),
  ]);
  return { state, chapters, selected, log };
}

const noop = () => undefined;
const later = async () => undefined;

/** A rendered component, with the source file that defines it. */
interface Screen {
  file: string;
  name: string;
  element: ReactElement;
}

/** Each component the window renders, with the props App gives it. */
function components({ state, chapters, selected, log }: Snapshot): Screen[] {
  const list: Screen[] = [
    { file: 'App.tsx', name: 'App', element: createElement(App, { api: {} as Api, initial: state }) },
    {
      file: 'SourceBar.tsx',
      name: 'SourceBar',
      element: createElement(SourceBar, {
        state,
        onLoad: noop,
        readClipboard: () => Promise.resolve(''),
        onClear: noop,
        onOptions: later,
        onPick: later,
        onError: noop,
        loadRefusal: '',
        optionError: '',
        onOpenSettings: noop,
        settingsOpen: false,
      }),
    },
    {
      file: 'ChapterPanel.tsx',
      name: 'ChapterPanel',
      element: createElement(ChapterPanel, {
        state,
        chapters,
        selected,
        disabled: state.loading || state.running,
        onToggle: later,
        onToggleMany: later,
        onSetAll: later,
        onRange: () => Promise.resolve(0),
        onVolumes: later,
        onReload: noop,
        onError: noop,
      }),
    },
    {
      file: 'OptionsPanel.tsx',
      name: 'OptionsPanel',
      element: createElement(OptionsPanel, { state, onOptions: later, onChooseFolder: later, onError: noop }),
    },
    {
      file: 'ActivityPanel.tsx',
      name: 'ActivityPanel',
      element: createElement(ActivityPanel, { state, onOpenFolder: noop }),
    },
    {
      file: 'DownloadDock.tsx',
      name: 'DownloadDock',
      element: createElement(DownloadDock, { state, error: '', chapters, selected, onStart: noop, onCancel: noop }),
    },
    {
      file: 'LogPanel.tsx',
      name: 'StatusBar',
      element: createElement(StatusBar, {
        log,
        version: state.version,
        releaseURL: state.releaseURL,
        update: state.update,
        open: false,
        onToggle: noop,
        onOpenURL: noop,
        onUpdate: later,
      }),
    },
    { file: 'LogPanel.tsx', name: 'LogDrawer', element: createElement(LogDrawer, { log, onClose: noop }) },
    {
      file: 'SettingsDialog.tsx',
      name: 'SettingsDialog',
      element: createElement(SettingsDialog, {
        state,
        onClose: noop,
        onCheckUpdates: noop,
        onOpenURL: noop,
        onError: noop,
      }),
    },
    {
      file: 'SettingsDialog.tsx',
      name: 'SettingsDialog without updates',
      element: createElement(SettingsDialog, {
        state: { ...state, releaseURL: '' },
        onClose: noop,
        onCheckUpdates: noop,
        onOpenURL: noop,
        onError: noop,
      }),
    },
    {
      file: 'fields.tsx',
      name: 'TextField',
      element: createElement(TextField, {
        label: t('source.url.label'),
        value: '',
        placeholder: t('source.url.placeholder'),
        hint: t('source.filter.noLanguage'),
        onCommit: later,
        onError: noop,
      }),
    },
    {
      file: 'fields.tsx',
      name: 'SelectField',
      element: createElement(SelectField, {
        label: t('source.filter.language'),
        value: '',
        options: [{ value: '', label: t('source.filter.any') }],
        onChange: noop,
      }),
    },
    {
      file: 'fields.tsx',
      name: 'Switch',
      element: createElement(Switch, {
        label: t('settings.updates.check'),
        description: t('settings.updates.hint'),
        checked: true,
        onChange: noop,
      }),
    },
    {
      file: 'fields.tsx',
      name: 'Segmented',
      element: createElement(Segmented, {
        label: t('settings.language.heading'),
        value: 'system',
        options: [
          { value: 'system', label: t('settings.language.system') },
          { value: 'en', label: LANG_INFO.en.name },
        ],
        onChange: noop,
      }),
    },
    {
      file: 'fields.tsx',
      name: 'Check',
      element: createElement(Check, { label: t('common.all'), checked: false, onChange: noop }),
    },
  ];
  if (state.update?.oneClick) {
    list.push({
      file: 'UpdateDialog.tsx',
      name: 'UpdateDialog',
      element: createElement(UpdateDialog, {
        latest: state.update.latest,
        current: state.version,
        onUpdate: later,
        onReleaseNotes: noop,
        onClose: noop,
      }),
    });
  }
  if (state.browserAlert !== '') {
    list.push({
      file: 'BrowserAlert.tsx',
      name: 'BrowserAlert',
      element: createElement(BrowserAlert, {
        message: say(t, state.browserAlertMsg, state.browserAlert),
        onShow: noop,
        onDismiss: noop,
      }),
    });
  }
  return list;
}

function render(element: ReactElement): string {
  return renderToStaticMarkup(
    createElement(
      I18nProvider,
      { pref: 'en', persist: async () => {}, catalog: pseudo, systemLanguages: ['en-US'] },
      element,
    ),
  );
}

// Every component file in src that reads its strings through useI18n; each must be rendered above.
const callers = Object.entries(
  import.meta.glob('../*.tsx', { query: '?raw', import: 'default', eager: true }) as Record<string, string>,
)
  .filter(([, text]) => text.includes('useI18n()'))
  .map(([path]) => path.replace(/^.*\//, ''));

describe('the pseudo catalog', () => {
  it('wraps every English value and keeps its placeholders and tags', () => {
    const values = Object.entries(CATALOG.en ?? {});
    expect(values.length).toBeGreaterThan(200);
    for (const [key, value] of values) {
      expect(pseudo.en?.[key], key).toBe(`⟦${value}⟧`);
    }
  });
});

describe('the rendered window in the pseudo catalog', () => {
  it('renders every component file that calls useI18n', async () => {
    expect(callers.length).toBeGreaterThan(5);
    const covered = new Set<string>();
    for (const [, setup] of SCREENS) {
      for (const { file } of components(await snapshot(setup))) covered.add(file);
    }
    expect(callers.filter((file) => !covered.has(file))).toEqual([]);
  });

  for (const [name, setup] of SCREENS) {
    it(`shows no English that skips t() in the ${name} state`, async () => {
      const screen = await snapshot(setup);
      for (const { name: component, element } of components(screen)) {
        const html = render(element);
        expect(html, component).toContain('⟦');
        const text = untranslated(html);
        const leaked = englishLeaks.filter((english) => new RegExp(`\\b${escapeRegExp(english)}\\b`).test(text));
        expect(leaked, `${component} in ${name}`).toEqual([]);
      }
    });
  }
});
