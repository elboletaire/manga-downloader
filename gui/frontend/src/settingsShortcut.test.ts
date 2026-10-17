import { afterEach, describe, expect, it, vi } from 'vitest';
import { isMac, isSettingsShortcut, shortcutLabel } from './settingsShortcut';

type Key = Parameters<typeof isSettingsShortcut>[0];

const none: Key = { key: ',', ctrlKey: false, metaKey: false, shiftKey: false, altKey: false };

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('isMac', () => {
  it('reads the platform string it is given', () => {
    expect(isMac('MacIntel')).toBe(true);
    expect(isMac('macOS')).toBe(true);
    expect(isMac('Win32')).toBe(false);
    expect(isMac('Linux x86_64')).toBe(false);
  });

  it('reads userAgentData before navigator.platform', () => {
    vi.stubGlobal('navigator', { userAgentData: { platform: 'macOS' }, platform: 'Win32' });
    expect(isMac()).toBe(true);
  });

  it('falls back to navigator.platform', () => {
    vi.stubGlobal('navigator', { platform: 'MacIntel' });
    expect(isMac()).toBe(true);
    vi.stubGlobal('navigator', { platform: 'Linux x86_64' });
    expect(isMac()).toBe(false);
  });

  it('is false with no navigator', () => {
    vi.stubGlobal('navigator', undefined);
    expect(isMac()).toBe(false);
  });
});

describe('isSettingsShortcut', () => {
  it('is Ctrl+, off macOS', () => {
    expect(isSettingsShortcut({ ...none, ctrlKey: true }, false)).toBe(true);
    expect(isSettingsShortcut({ ...none, metaKey: true }, false)).toBe(false);
    expect(isSettingsShortcut(none, false)).toBe(false);
  });

  it('is Cmd+, on macOS', () => {
    expect(isSettingsShortcut({ ...none, metaKey: true }, true)).toBe(true);
    expect(isSettingsShortcut({ ...none, ctrlKey: true }, true)).toBe(false);
    expect(isSettingsShortcut(none, true)).toBe(false);
  });

  it('rejects the other modifier held along with the shortcut key', () => {
    expect(isSettingsShortcut({ ...none, ctrlKey: true, metaKey: true }, false)).toBe(false);
    expect(isSettingsShortcut({ ...none, ctrlKey: true, metaKey: true }, true)).toBe(false);
  });

  it('rejects Shift and Alt combinations', () => {
    expect(isSettingsShortcut({ ...none, ctrlKey: true, shiftKey: true }, false)).toBe(false);
    expect(isSettingsShortcut({ ...none, ctrlKey: true, altKey: true }, false)).toBe(false);
    expect(isSettingsShortcut({ ...none, metaKey: true, shiftKey: true }, true)).toBe(false);
    expect(isSettingsShortcut({ ...none, metaKey: true, altKey: true }, true)).toBe(false);
  });

  it('rejects any other key', () => {
    expect(isSettingsShortcut({ ...none, key: '.', ctrlKey: true }, false)).toBe(false);
    expect(isSettingsShortcut({ ...none, key: 'p', metaKey: true }, true)).toBe(false);
  });
});

describe('shortcutLabel', () => {
  it('shows the macOS and other-platform labels', () => {
    expect(shortcutLabel(true)).toBe('⌘,');
    expect(shortcutLabel(false)).toBe('Ctrl+,');
  });
});
