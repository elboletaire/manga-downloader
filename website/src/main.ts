// Progressive enhancement only: the page works fully without this script.
import '@fontsource-variable/inter/wght.css'
import '@fontsource-variable/fredoka/wght.css'
import './styles.css'
import { EN_RUNTIME, plural, type RuntimeStrings } from './i18n'
import { LANG_KEY, withHash } from './lang'
import { forcedTheme, parseTheme, schemeMedia, THEME_KEY, type Theme } from './theme'
import {
  LATEST_RELEASE_API,
  chooseDownload,
  detectPlatform,
  resolveAssets,
  type Platform,
  type PlatformHints,
  type Release,
} from './release'

interface UADataLike {
  platform?: string
  getHighEntropyValues?: (hints: string[]) => Promise<{ architecture?: string; bitness?: string }>
}

const CACHE_KEY = 'md:latest-release'
const CACHE_TTL = 10 * 60 * 1000

/** This page's runtime strings, embedded by the site-data plugin (vite.config.ts). */
function runtimeStrings(): RuntimeStrings {
  try {
    const text = document.getElementById('i18n-runtime')?.textContent
    if (text) return JSON.parse(text) as RuntimeStrings
  } catch {
    // Missing or corrupt: fall back to English.
  }
  return EN_RUNTIME
}

/** The latest release embedded at build time. Provides a fallback for download
 * links when the browser's GitHub API request is rate-limited. */
function embeddedRelease(): Release | null {
  try {
    const text = document.getElementById('latest-release')?.textContent
    if (!text) return null
    const data = JSON.parse(text) as unknown
    // Validate shape: tag_name is required, assets must be an array
    if (typeof data === 'object' && data !== null && 'tag_name' in data && typeof (data as any).tag_name === 'string' && Array.isArray((data as any).assets)) {
      return data as Release
    }
  } catch {
    // Missing or corrupt: will fetch from the API instead.
  }
  return null
}

const STRINGS = runtimeStrings()
const LANG = document.documentElement.lang || 'en'

async function platformHints(): Promise<PlatformHints> {
  const uaData = (navigator as Navigator & { userAgentData?: UADataLike }).userAgentData
  const hints: PlatformHints = { userAgent: navigator.userAgent, uaPlatform: uaData?.platform }
  if (uaData?.getHighEntropyValues) {
    try {
      Object.assign(hints, await withTimeout(uaData.getHighEntropyValues(['architecture', 'bitness']), 500))
    } catch {
      // Not available or too slow: the user agent string is good enough.
    }
  }
  return hints
}

function withTimeout<T>(p: Promise<T>, ms: number): Promise<T> {
  return Promise.race([p, new Promise<never>((_, reject) => setTimeout(() => reject(new Error('timeout')), ms))])
}

/** Fetch the latest release from GitHub. Returns null if the API can't be reached.
 * Result is cached for a few minutes (the unauthenticated API allows 60 calls an hour).
 * If an embedded release was built into the page, a fetch may still return a newer one. */
async function latestRelease(): Promise<Release | null> {
  try {
    const cached = JSON.parse(sessionStorage.getItem(CACHE_KEY) ?? 'null') as { at: number; release: Release } | null
    if (cached && Date.now() - cached.at < CACHE_TTL) return cached.release
  } catch {
    // Storage disabled or corrupt: just fetch.
  }
  try {
    const res = await fetch(LATEST_RELEASE_API, {
      headers: { Accept: 'application/vnd.github+json' },
      signal: AbortSignal.timeout(6000),
    })
    if (!res.ok) return null
    const release = (await res.json()) as Release
    if (typeof release.tag_name !== 'string' || !Array.isArray(release.assets)) return null
    try {
      sessionStorage.setItem(CACHE_KEY, JSON.stringify({ at: Date.now(), release }))
    } catch {
      // Ignore quota or privacy-mode errors.
    }
    return release
  } catch {
    return null
  }
}

function renderDownload(release: Release | null, platform: Platform, settled: boolean): void {
  const root = document.querySelector<HTMLElement>('[data-download]')
  if (!root) return
  const link = root.querySelector<HTMLAnchorElement>('[data-download-link]')!
  const label = root.querySelector<HTMLElement>('[data-download-label]')!
  const detail = root.querySelector<HTMLElement>('[data-download-detail]')!

  const choice = chooseDownload(release, platform, STRINGS.download)
  link.href = choice.href
  link.dataset.kind = choice.kind
  label.textContent = choice.label
  // Until the API answers, keep the neutral line instead of flashing a fallback.
  if (settled) detail.textContent = choice.detail
}

function renderDownloads(release: Release | null): void {
  const rows = new Map(resolveAssets(release).map((r) => [r.key, r]))
  for (const link of document.querySelectorAll<HTMLAnchorElement>('[data-asset]')) {
    const row = rows.get(link.dataset.asset!)
    if (!row) continue
    link.href = row.href
    if (!release) continue
    if (row.available) {
      link.dataset.state = 'ready'
      link.title = [row.name, row.size].filter(Boolean).join(' · ')
    }
  }
  const tag = document.querySelector<HTMLAnchorElement>('[data-release-tag]')
  if (tag && release) {
    tag.textContent = release.tag_name
    if (release.html_url) tag.href = release.html_url
  }
}

async function enhanceDownload(): Promise<void> {
  const hints = await platformHints()
  const platform = detectPlatform(hints)
  document.documentElement.dataset.os = platform.os
  if (platform.os !== 'unknown') renderDownload(null, platform, false)

  // If a release was embedded at build time, render it immediately as settled
  const embedded = embeddedRelease()
  if (embedded) {
    renderDownload(embedded, platform, true)
    renderDownloads(embedded)
  }

  // Fetch the latest release (may be newer than the embedded one)
  // If the fetch fails, keep the embedded version (or fallback to null)
  const release = (await latestRelease()) ?? embedded
  if (!embedded || release !== embedded) {
    // Only re-render if we have a newer release or no embedded one
    renderDownload(release, platform, true)
    renderDownloads(release)
  }
}

// Copy buttons ---------------------------------------------------------------

const COPY_ICON =
  '<svg class="icon-copy" viewBox="0 0 24 24" aria-hidden="true"><rect x="8" y="8" width="12" height="12" rx="2.5" fill="none" stroke="currentColor" stroke-width="2"/><path d="M16 8V6.5A2.5 2.5 0 0 0 13.5 4h-7A2.5 2.5 0 0 0 4 6.5v7A2.5 2.5 0 0 0 6.5 16H8" fill="none" stroke="currentColor" stroke-width="2"/></svg>' +
  '<svg class="icon-done" viewBox="0 0 24 24" aria-hidden="true"><path d="m5 12.5 4.5 4.5L19 7.5" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"/></svg>'

function enhanceCopy(): void {
  if (!navigator.clipboard) return
  for (const block of document.querySelectorAll<HTMLElement>('[data-copy]')) {
    const code = block.querySelector('code')
    if (!code) continue
    const button = document.createElement('button')
    button.type = 'button'
    button.className = 'copy'
    button.setAttribute('aria-label', STRINGS.copy.label)
    button.innerHTML = COPY_ICON
    let timer: number | undefined
    button.addEventListener('click', async () => {
      try {
        await navigator.clipboard.writeText(code.textContent ?? '')
        button.classList.add('is-done')
        button.setAttribute('aria-label', STRINGS.copy.done)
        clearTimeout(timer)
        timer = window.setTimeout(() => {
          button.classList.remove('is-done')
          button.setAttribute('aria-label', STRINGS.copy.label)
        }, 1800)
      } catch {
        // Clipboard denied: the text is still selectable.
      }
    })
    block.classList.add('has-copy')
    block.append(button)
  }
}

// Sites filter ---------------------------------------------------------------

function enhanceSites(): void {
  const tools = document.querySelector<HTMLElement>('[data-sites-tools]')
  const input = document.querySelector<HTMLInputElement>('[data-sites-filter]')
  const browserOnly = document.querySelector<HTMLInputElement>('[data-sites-browser]')
  const count = document.querySelector<HTMLElement>('[data-sites-count]')
  const empty = document.querySelector<HTMLElement>('[data-sites-empty]')
  const items = [...document.querySelectorAll<HTMLLIElement>('[data-sites] > li')]
  if (!tools || !input || !browserOnly || !count || !empty || items.length === 0) return

  const update = () => {
    const terms = input.value.toLowerCase().split(/\s+/).filter(Boolean)
    let shown = 0
    for (const li of items) {
      const haystack = li.dataset.search ?? ''
      const match =
        terms.every((t) => haystack.includes(t)) && (!browserOnly.checked || li.hasAttribute('data-browser'))
      li.hidden = !match
      if (match) shown++
    }
    count.textContent =
      shown === items.length
        ? plural(STRINGS.sites, 'all', items.length, LANG)
        : plural(STRINGS.sites, 'filtered', items.length, LANG, { shown })
    empty.hidden = shown > 0
  }

  input.addEventListener('input', update)
  browserOnly.addEventListener('change', update)
  tools.hidden = false
  update()
}

/** Remembers the language picked in the header, so the English page stops redirecting. */
function rememberLanguage(): void {
  for (const link of document.querySelectorAll<HTMLAnchorElement>('a[data-lang]')) {
    link.addEventListener('click', () => {
      try {
        localStorage.setItem(LANG_KEY, link.dataset.lang!)
      } catch {
        // Storage disabled: the link still works.
      }
    })
  }
}

/** Keeps the language links pointing at the current #section (ids match across languages), so new-tab opens carry it too. */
function syncLanguageLinks(): void {
  for (const link of document.querySelectorAll<HTMLAnchorElement>('a[data-lang]')) {
    link.setAttribute('href', withHash(link.getAttribute('href') ?? '', location.hash))
  }
}

/** Opens the FAQ entry a link or the URL points at; the browser only expands ancestors. */
function openTargetedDetails(): void {
  const id = decodeURIComponent(location.hash.slice(1))
  const el = id ? document.getElementById(id) : null
  if (el instanceof HTMLDetailsElement) {
    el.open = true
    el.scrollIntoView()
  }
}

// Theme switch ---------------------------------------------------------------

/** Points light/dark <picture> sources and theme-color metas at the chosen theme. */
function applyMedia(forced: 'light' | 'dark' | null): void {
  // A rewritten media stops matching the first two selectors, so `data-media` keeps those nodes found.
  const nodes = document.querySelectorAll<HTMLElement>(
    'source[media*="prefers-color-scheme"], meta[name="theme-color"][media*="prefers-color-scheme"], [data-media]',
  )
  for (const node of nodes) {
    const original = (node.dataset.media ??= node.getAttribute('media') ?? '')
    const scheme = original.includes('dark') ? 'dark' : 'light'
    node.setAttribute('media', schemeMedia(original, scheme, forced))
  }
  // Browsers may skip re-selecting a <picture> source when its media changes; re-inserting the <img> forces it.
  for (const img of document.querySelectorAll<HTMLImageElement>('picture > img')) {
    img.parentNode!.insertBefore(img, img.nextSibling)
  }
}

function enhanceTheme(): void {
  const groups = [...document.querySelectorAll<HTMLFieldSetElement>('[data-theme-switch]')]
  let stored: string | null = null
  try {
    stored = localStorage.getItem(THEME_KEY)
  } catch {
    // Storage disabled: the choice lasts until the page closes.
  }
  const apply = (choice: Theme) => {
    const forced = forcedTheme(choice)
    if (forced) document.documentElement.setAttribute('data-theme', forced)
    else document.documentElement.removeAttribute('data-theme')
    applyMedia(forced)
    for (const input of document.querySelectorAll<HTMLInputElement>('[data-theme-switch] input')) {
      input.checked = input.value === choice
    }
  }
  apply((forcedTheme(stored) ?? 'system') as Theme)
  for (const group of groups) {
    group.hidden = false
    group.addEventListener('change', (e) => {
      const choice = parseTheme((e.target as HTMLInputElement).value)
      if (!choice) return
      apply(choice)
      try {
        if (choice === 'system') localStorage.removeItem(THEME_KEY)
        else localStorage.setItem(THEME_KEY, choice)
      } catch {
        // Storage disabled: the switch still works for this visit.
      }
    })
  }
}

enhanceTheme()
enhanceCopy()
enhanceSites()
openTargetedDetails()
window.addEventListener('hashchange', openTargetedDetails)
rememberLanguage()
syncLanguageLinks()
window.addEventListener('hashchange', syncLanguageLinks)
void enhanceDownload()
