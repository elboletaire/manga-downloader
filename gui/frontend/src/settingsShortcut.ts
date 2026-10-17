// The keyboard shortcut that opens the settings dialog: Ctrl+, on Windows and
// Linux, Cmd+, on macOS, as those platforms' apps use for preferences.

/** Whether the platform is macOS; reads the browser's platform unless given one */
export function isMac(platform?: string): boolean {
  return /^mac/i.test(platform ?? browserPlatform());
}

function browserPlatform(): string {
  const nav = globalThis.navigator as (Navigator & { userAgentData?: { platform?: string } }) | undefined;
  return nav?.userAgentData?.platform ?? nav?.platform ?? '';
}

/** Whether a keydown is the settings shortcut; the other modifier must be up */
export function isSettingsShortcut(
  e: { key: string; ctrlKey: boolean; metaKey: boolean; shiftKey: boolean; altKey: boolean },
  mac: boolean,
): boolean {
  if (e.key !== ',' || e.shiftKey || e.altKey) {
    return false;
  }
  return mac ? e.metaKey && !e.ctrlKey : e.ctrlKey && !e.metaKey;
}

/** The shortcut as shown in the UI */
export function shortcutLabel(mac: boolean): string {
  return mac ? '⌘,' : 'Ctrl+,';
}

/** Whether a keydown opens or closes the settings; never over another modal (the browser alert or the update dialog) */
export function shouldToggleSettings(
  e: { key: string; ctrlKey: boolean; metaKey: boolean; shiftKey: boolean; altKey: boolean },
  mac: boolean,
  otherModalOpen: boolean,
): boolean {
  return !otherModalOpen && isSettingsShortcut(e, mac);
}
