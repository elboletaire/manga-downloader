// The external links the window opens in the system's browser, and the click handler that opens them.

import type { MouseEvent } from 'react';

export const ISSUES_URL = 'https://github.com/elboletaire/manga-downloader/issues';
export const LICENSE_URL = 'https://github.com/elboletaire/manga-downloader/blob/master/LICENSE';
export const KOFI_URL = 'https://ko-fi.com/elboletaire';

// a link opens outside the window, which must stay on the app
export function linkClick(url: string, openURL: (url: string) => void) {
  return (e: MouseEvent<HTMLAnchorElement>) => {
    e.preventDefault();
    openURL(url);
  };
}
