// Progressive enhancement only: the page works fully without this script.
import '@fontsource-variable/inter/wght.css'
import '@fontsource-variable/fredoka/wght.css'
import './styles.css'
import { EN_RUNTIME, plural, type RuntimeStrings } from './i18n'
import { LANG_KEY } from './lang'
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

/** The latest release, or null if GitHub can't be reached. Cached for a few
 * minutes, since the unauthenticated API allows 60 calls an hour. */
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

  const release = await latestRelease()
  renderDownload(release, platform, true)
  renderDownloads(release)
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

enhanceCopy()
enhanceSites()
rememberLanguage()
void enhanceDownload()
