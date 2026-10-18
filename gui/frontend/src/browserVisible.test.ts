import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { BrowserAlert } from './BrowserAlert';
import { browserSwitchOf } from './browserVisible';
import { createT } from './i18n';
import { createMockApi } from './mock';
import { OptionsPanel } from './OptionsPanel';
import type { State } from './types';

const en = createT('en');
const BROWSER_HINT = en('options.browser.hint');
const BROWSER_AUTO_HINT = en('options.browser.auto');

async function loaded(patch: Partial<State> = {}): Promise<State> {
  const base = await createMockApi({ scenario: 'loaded', freeze: true }).state();
  return { ...base, ...patch };
}

function options(state: State): string {
  return renderToStaticMarkup(
    createElement(OptionsPanel, {
      state,
      onOptions: () => Promise.resolve(),
      onChooseFolder: () => Promise.resolve(),
      onError: () => undefined,
    }),
  );
}

/** The browser switch's input, from the rendered options */
function browserInput(markup: string): string {
  const at = markup.indexOf('Show the browser window');
  const input = markup.slice(at).match(/<input[^>]*role="switch"[^>]*\/>/);
  return input?.[0] ?? '';
}

describe('browserSwitchOf', () => {
  it('translates the hints', async () => {
    expect(browserSwitchOf(await loaded({ browserVisibleAuto: true }), createT('es')).hint).toBe('Hace falta para este sitio: la ventana se abre sola.');
    expect(browserSwitchOf(await loaded(), createT('ca')).hint).toBe("Gairebé mai cal: la finestra s'obre sola quan un lloc la demana.");
  });

  it('follows the option when the site needs no window', async () => {
    const state = await loaded();
    expect(browserSwitchOf(state, en)).toEqual({ checked: false, disabled: false, hint: BROWSER_HINT });
    const on = { ...state, options: { ...state.options, browserVisible: true } };
    expect(browserSwitchOf(on, en).checked).toBe(true);
    expect(browserSwitchOf({ ...state, running: true }, en).disabled).toBe(true);
  });

  it('shows on and locked, with the hint, when the site needs the window', async () => {
    const state = await loaded({ browserVisibleAuto: true });
    expect(state.options.browserVisible).toBe(false);
    expect(browserSwitchOf(state, en)).toEqual({ checked: true, disabled: true, hint: BROWSER_AUTO_HINT });
  });
});

describe('the browser switch in the options', () => {
  it('renders the option as it is', async () => {
    const markup = options(await loaded());
    expect(markup).toContain(BROWSER_HINT);
    const input = browserInput(markup);
    expect(input).not.toContain('checked');
    expect(input).not.toContain('disabled');
  });

  it('renders checked and disabled with the hint for a site that needs it', async () => {
    const markup = options(await loaded({ browserVisibleAuto: true }));
    expect(markup).toContain(BROWSER_AUTO_HINT);
    const input = browserInput(markup);
    expect(input).toContain('checked');
    expect(input).toContain('disabled');
  });
});

describe('BrowserAlert', () => {
  it('shows the message and both actions', () => {
    const markup = renderToStaticMarkup(
      createElement(BrowserAlert, {
        message: 'Brave opened a window',
        onShow: () => undefined,
        onDismiss: () => undefined,
      }),
    );
    expect(markup).toContain('role="alertdialog"');
    expect(markup).toContain('Brave opened a window');
    expect(markup).toContain('Show the browser window');
    expect(markup).toContain('Dismiss');
  });
});

describe('the mock browser alert', () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it('starts with the alert in the check scenario, until dismissed', async () => {
    const api = createMockApi({ scenario: 'check', freeze: true });
    const state = await api.state();
    expect(state.browserVisibleAuto).toBe(true);
    expect(state.browserAlert).toContain('opened a window to pass a check');

    await api.showBrowser();
    expect((await api.state()).browserAlert).not.toBe('');
    await api.dismissBrowserAlert();
    const after = await api.state();
    expect(after.browserAlert).toBe('');
    expect(after.browserVisibleAuto).toBe(true);
  });

  it('shows the alert while a site that needs the window loads, and hides it once loaded', async () => {
    vi.useFakeTimers();
    const api = createMockApi('empty');
    const loading = api.load('https://check.example.com/series/a');
    expect((await api.state()).browserAlert).not.toBe('');
    await vi.runAllTimersAsync();
    await loading;
    const state = await api.state();
    expect(state.loaded).toBe(true);
    expect(state.browserAlert).toBe('');
    expect(state.browserVisibleAuto).toBe(true);

    await api.clear();
    expect((await api.state()).browserVisibleAuto).toBe(false);
  });
});
