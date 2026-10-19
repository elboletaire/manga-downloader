import { describe, expect, it } from 'vitest';
import { bootView, nextView } from './startView';
import type { State } from './types';

type ViewState = Parameters<typeof bootView>[0];

const fresh = (over: Partial<ViewState> = {}, simpleStart = true): ViewState => ({
  prefs: { checkUpdates: true, simpleStart },
  url: '',
  loading: false,
  loaded: false,
  running: false,
  summary: null,
  ...over,
});

const URL_ = 'https://example.com/manga/x';

describe('bootView', () => {
  it('opens on the simple view when the pref is on and nothing is going on', () => {
    expect(bootView(fresh())).toBe('simple');
  });

  it('opens on the full view when the pref is off', () => {
    expect(bootView(fresh({}, false))).toBe('full');
  });

  it('opens on the full view when the window has something to show', () => {
    expect(bootView(fresh({ loading: true, url: URL_ }))).toBe('full');
    expect(bootView(fresh({ loaded: true, url: URL_ }))).toBe('full');
    expect(bootView(fresh({ running: true }))).toBe('full');
    expect(bootView(fresh({ summary: {} as State['summary'] }))).toBe('full');
    // a failed load keeps its URL, and its error belongs to the full view
    expect(bootView(fresh({ url: URL_ }))).toBe('full');
  });
});

describe('nextView', () => {
  it('stays simple while nothing has started', () => {
    expect(nextView('simple', fresh(), false)).toBe('simple');
  });

  it('switches to the full view once a load starts', () => {
    expect(nextView('simple', fresh({ loading: true, url: URL_ }), false)).toBe('full');
  });

  it('switches to the full view when the pref is turned off while empty', () => {
    expect(nextView('simple', fresh({}, false), false)).toBe('full');
  });

  it('switches back to the simple view when the pref is turned on while empty', () => {
    expect(nextView('full', fresh(), false)).toBe('simple');
  });

  it('never goes back to the simple view once a load has started, even when cleared', () => {
    expect(nextView('full', fresh(), true)).toBe('full');
  });

  it('ignores the pref once a load has started', () => {
    expect(nextView('full', fresh({}, true), true)).toBe('full');
    expect(nextView('full', fresh({ loaded: true, url: URL_ }, false), true)).toBe('full');
  });
});
