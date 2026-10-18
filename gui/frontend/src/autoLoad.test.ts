import { describe, expect, it } from 'vitest';
import { isSeriesUrl, shouldAutoLoad } from './autoLoad';

const SERIES = 'https://example.com/series/the-cartographer';
const idle = { url: '', busy: false };

describe('isSeriesUrl', () => {
  it('accepts http and https URLs to a series path', () => {
    expect(isSeriesUrl(SERIES)).toBe(true);
    expect(isSeriesUrl('https://mangadex.org/title/abc-123/one-piece')).toBe(true);
    expect(isSeriesUrl('https://example.com:8443/a?b=c')).toBe(true);
  });

  it('refuses a bare host, which is not a series', () => {
    expect(isSeriesUrl('http://example.com')).toBe(false);
    expect(isSeriesUrl('https://e')).toBe(false);
    expect(isSeriesUrl('https://mangadex')).toBe(false);
    expect(isSeriesUrl('https://mangadex.org')).toBe(false);
    expect(isSeriesUrl('https://mangadex.org/')).toBe(false);
  });

  it('accepts a bare host when it was pasted', () => {
    expect(isSeriesUrl('https://kaijuno8chapters.com/', { pasted: true })).toBe(true);
    expect(isSeriesUrl('https://w2.dandadanmanga.net', { pasted: true })).toBe(true);
    expect(isSeriesUrl('https://mangadex', { pasted: true })).toBe(false);
  });

  it('refuses hosts with empty labels', () => {
    expect(isSeriesUrl('https://a./x')).toBe(false);
    expect(isSeriesUrl('https://.com/x')).toBe(false);
  });

  it('refuses text that is not a URL', () => {
    expect(isSeriesUrl('')).toBe(false);
    expect(isSeriesUrl('one piece')).toBe(false);
    expect(isSeriesUrl('example.com/series/x')).toBe(false);
    expect(isSeriesUrl('ftp://example.com/x')).toBe(false);
    expect(isSeriesUrl('javascript:alert(1)')).toBe(false);
    expect(isSeriesUrl('https://')).toBe(false);
    expect(isSeriesUrl('https:example.com')).toBe(false);
    expect(isSeriesUrl('https:///example.com')).toBe(false);
    expect(isSeriesUrl('https://exa mple.com')).toBe(false);
  });
});

describe('shouldAutoLoad', () => {
  it('does not load non-URL text', () => {
    expect(shouldAutoLoad('', idle)).toBe(false);
    expect(shouldAutoLoad('   ', idle)).toBe(false);
    expect(shouldAutoLoad('the cartographer', idle)).toBe(false);
  });

  it('does not load a bare host', () => {
    expect(shouldAutoLoad('https://mangadex.org', idle)).toBe(false);
  });

  it('loads a pasted bare host, which a typed one never does', () => {
    expect(shouldAutoLoad('https://kaijuno8chapters.com/', idle, { pasted: true })).toBe(true);
    expect(shouldAutoLoad('https://w2.dandadanmanga.net', idle, { pasted: true })).toBe(true);
    expect(shouldAutoLoad('https://kaijuno8chapters.com/', idle)).toBe(false);
    expect(shouldAutoLoad('https://kaijuno8chapters.com/', idle, { pasted: false })).toBe(false);
  });

  it('does not reload a pasted bare host that is already loaded', () => {
    const loaded = { url: 'https://kaijuno8chapters.com/', busy: false };
    expect(shouldAutoLoad('https://kaijuno8chapters.com/', loaded, { pasted: true })).toBe(false);
    expect(shouldAutoLoad('  https://kaijuno8chapters.com/\n', loaded, { pasted: true })).toBe(false);
  });

  it('does not load a pasted bare host while busy or when it is not a URL', () => {
    expect(shouldAutoLoad('https://kaijuno8chapters.com/', { url: '', busy: true }, { pasted: true })).toBe(false);
    expect(shouldAutoLoad('one piece', idle, { pasted: true })).toBe(false);
  });

  it('does not load the URL that is loaded or loading', () => {
    expect(shouldAutoLoad(SERIES, { url: SERIES, busy: false })).toBe(false);
    expect(shouldAutoLoad(SERIES, { url: SERIES, busy: true })).toBe(false);
  });

  it('compares the trimmed value with the loaded URL', () => {
    expect(shouldAutoLoad(`  ${SERIES}\n`, { url: SERIES, busy: false })).toBe(false);
    expect(shouldAutoLoad(`  ${SERIES}  `, { url: `${SERIES} `, busy: false })).toBe(false);
  });

  it('does not load while a load or a download is running', () => {
    expect(shouldAutoLoad(SERIES, { url: '', busy: true })).toBe(false);
    expect(shouldAutoLoad('https://example.com/series/other', { url: SERIES, busy: true })).toBe(false);
  });

  it('loads a valid new URL', () => {
    expect(shouldAutoLoad(SERIES, idle)).toBe(true);
    expect(shouldAutoLoad(SERIES, { url: 'https://example.com/series/other', busy: false })).toBe(true);
  });

  it('loads a new URL with surrounding whitespace, trimmed', () => {
    expect(shouldAutoLoad(`  ${SERIES}  `, idle)).toBe(true);
    expect(shouldAutoLoad(`\t${SERIES}\n`, { url: '', busy: false })).toBe(true);
  });

  it('does not retry the same URL after it failed to load', () => {
    // a failed load keeps the URL it was started with, not loaded: still the same
    const failed = { url: 'https://unknown.example.org/manga/x', busy: false };
    expect(shouldAutoLoad('https://unknown.example.org/manga/x', failed)).toBe(false);
    expect(shouldAutoLoad('  https://unknown.example.org/manga/x ', failed)).toBe(false);
    // an edit to another URL does load
    expect(shouldAutoLoad(SERIES, failed)).toBe(true);
  });
});
