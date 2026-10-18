// When the URL field loads by itself: once the typed value has paused and it
// is a series URL nobody has loaded yet. The decision is pure so it can be
// tested without a window; SourceBar owns the timer.

/** Quiet time after the last change before a pasted or typed URL loads */
export const AUTO_LOAD_DELAY_MS = 200;

/**
 * Whether text is an http(s) URL to a series: a dotted host with no empty
 * labels, and a path (a bare host only when pasted). Parsed with `new URL`.
 */
export function isSeriesUrl(text: string, { pasted = false }: { pasted?: boolean } = {}): boolean {
  // the scheme must be followed by `//` and then something that isn't a slash,
  // so `https:example.com` and `https:///example.com` are refused
  if (!/^https?:\/\/[^/]/i.test(text)) {
    return false;
  }
  try {
    const url = new URL(text);
    const labels = url.hostname.split('.');
    return (
      (url.protocol === 'http:' || url.protocol === 'https:') &&
      labels.length > 1 &&
      labels.every(Boolean) &&
      (pasted || url.pathname.length > 1)
    );
  } catch {
    return false;
  }
}

/**
 * Whether the value should load now. url is the series the backend holds, set
 * when a load starts, so a loaded, loading or failed URL never reloads on its own.
 */
export function shouldAutoLoad(
  value: string,
  state: { url: string; busy: boolean },
  { pasted = false }: { pasted?: boolean } = {},
): boolean {
  if (state.busy) {
    return false;
  }
  const trimmed = value.trim();
  return isSeriesUrl(trimmed, { pasted }) && trimmed !== state.url.trim();
}
