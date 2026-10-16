// Pure logic behind the Download button: which OS the visitor is on, and
// which release asset matches it. No DOM access here, so it's all testable.

import { EN_RUNTIME, fill, type DownloadStrings } from './i18n'

export const REPO = 'elboletaire/manga-downloader'
export const RELEASES_URL = `https://github.com/${REPO}/releases/latest`
export const LATEST_RELEASE_API = `https://api.github.com/repos/${REPO}/releases/latest`

export type OS = 'windows' | 'macos' | 'linux' | 'unknown'
export type Arch = 'amd64' | 'arm64' | '386' | 'unknown'

export interface Platform {
  os: OS
  arch: Arch
}

/** What the browser tells us. Every field is optional: Safari and Firefox
 * don't implement `navigator.userAgentData`. */
export interface PlatformHints {
  userAgent?: string
  /** `navigator.userAgentData.platform`: "Windows", "macOS", "Linux"… */
  uaPlatform?: string
  /** High-entropy hints: "x86" / "arm", and "64" / "32". */
  architecture?: string
  bitness?: string
}

export interface ReleaseAsset {
  name: string
  browser_download_url: string
  size?: number
}

export interface Release {
  tag_name: string
  html_url?: string
  assets: ReleaseAsset[]
}

export const OS_NAMES: Record<Exclude<OS, 'unknown'>, string> = {
  windows: 'Windows',
  macos: 'macOS',
  linux: 'Linux',
}

export function detectPlatform(h: PlatformHints): Platform {
  const ua = h.userAgent ?? ''
  return { os: detectOS(h.uaPlatform, ua), arch: detectArch(h, ua) }
}

function detectOS(uaPlatform: string | undefined, ua: string): OS {
  const p = (uaPlatform ?? '').toLowerCase()
  if (p === 'windows') return 'windows'
  if (p === 'macos') return 'macos'
  if (p === 'linux') return 'linux'
  // Android, Chrome OS, iOS: nothing we ship runs there.
  if (p !== '') return 'unknown'

  if (/android|iphone|ipad|ipod|cros/i.test(ua)) return 'unknown'
  if (/windows/i.test(ua)) return 'windows'
  if (/macintosh|mac os x/i.test(ua)) return 'macos'
  if (/linux|x11/i.test(ua)) return 'linux'
  return 'unknown'
}

function detectArch(h: PlatformHints, ua: string): Arch {
  const a = (h.architecture ?? '').toLowerCase()
  if (a === 'arm') return h.bitness === '32' ? 'unknown' : 'arm64'
  if (a === 'x86') return h.bitness === '32' ? '386' : 'amd64'

  if (/aarch64|arm64/i.test(ua)) return 'arm64'
  if (/x86_64|x64|win64|wow64|amd64/i.test(ua)) return 'amd64'
  if (/i[3-6]86/i.test(ua)) return '386'
  return 'unknown'
}

/** Desktop app asset names for a platform, in order of preference. */
export function guiAssetNames(tag: string, p: Platform): string[] {
  const base = `manga-downloader-gui-${tag}`
  switch (p.os) {
    case 'windows':
      // The installer comes first; releases from before it fall back to the zip.
      // ARM Windows runs the x64 build emulated if there's no native one.
      return p.arch === 'arm64'
        ? [
            `${base}-windows-arm64-installer.exe`,
            `${base}-windows-amd64-installer.exe`,
            `${base}-windows-arm64.zip`,
            `${base}-windows-amd64.zip`,
          ]
        : [`${base}-windows-amd64-installer.exe`, `${base}-windows-amd64.zip`]
    case 'macos':
      return [`${base}-darwin-universal.zip`]
    case 'linux':
      return [`${base}-linux-${p.arch === 'arm64' ? 'arm64' : 'amd64'}.tar.gz`]
    default:
      return []
  }
}

export function findAsset(release: Release, names: string[]): ReleaseAsset | undefined {
  for (const name of names) {
    const asset = release.assets.find((a) => a.name === name)
    if (asset) return asset
  }
  return undefined
}

/** Human label for the architecture baked into an asset name. */
export function archLabel(assetName: string): string {
  if (/universal/.test(assetName)) return 'Universal'
  if (/arm64/.test(assetName)) return 'ARM64'
  if (/amd64/.test(assetName)) return 'x64'
  return ''
}

export function formatSize(bytes: number | undefined): string {
  if (!bytes) return ''
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

export interface DownloadChoice {
  /** `gui` links straight to the desktop app file; `page` to a releases page. */
  kind: 'gui' | 'page'
  href: string
  label: string
  detail: string
}

/**
 * Picks what the hero's Download button offers: always the desktop app for
 * the visitor's OS. `release` is null when the GitHub API couldn't be reached.
 * When the release has no matching file (yet), the button keeps its label and
 * links to the releases page.
 */
export function chooseDownload(release: Release | null, p: Platform, s: DownloadStrings = EN_RUNTIME.download): DownloadChoice {
  if (p.os === 'unknown') {
    return {
      kind: 'page',
      href: '#downloads',
      label: s.generic,
      detail: s.genericDetail,
    }
  }
  const osName = OS_NAMES[p.os]
  if (!release) {
    return {
      kind: 'page',
      href: RELEASES_URL,
      label: fill(s.forOS, { os: osName }),
      detail: s.latest,
    }
  }

  const tag = release.tag_name
  const gui = findAsset(release, guiAssetNames(tag, p))
  if (gui) {
    return {
      kind: 'gui',
      href: gui.browser_download_url,
      label: fill(s.forOS, { os: osName }),
      detail: joinDetail([fill(s.desktopApp, { tag }), archLabel(gui.name), formatSize(gui.size)]),
    }
  }

  return {
    kind: 'page',
    href: RELEASES_URL,
    label: fill(s.forOS, { os: osName }),
    detail: s.latest,
  }
}

function joinDetail(parts: string[]): string {
  return parts.filter(Boolean).join(' · ')
}

export interface AssetRow {
  /** Matches `data-asset` on the download links in the markup. */
  key: string
  href: string
  available: boolean
  name?: string
  size?: string
}

/** Every asset the page links to, keyed by the name without its tag. */
export const ASSET_KEYS = [
  'gui-windows-amd64-installer.exe',
  'gui-windows-arm64-installer.exe',
  'gui-windows-amd64.zip',
  'gui-windows-arm64.zip',
  'gui-darwin-universal.zip',
  'gui-linux-amd64.tar.gz',
  'gui-linux-arm64.tar.gz',
  'windows-amd64.zip',
  'windows-arm64.zip',
  'windows-32bit-legacy.zip',
  'darwin-arm64.tar.gz',
  'darwin-amd64.tar.gz',
  'linux-amd64.tar.gz',
  'linux-arm64.tar.gz',
  'linux-386.tar.gz',
] as const

export function assetNameForKey(tag: string, key: string): string {
  return key.startsWith('gui-')
    ? `manga-downloader-gui-${tag}-${key.slice(4)}`
    : `manga-downloader-${tag}-${key}`
}

/** Resolves every key against a release; missing ones point at the releases page. */
export function resolveAssets(release: Release | null, keys: readonly string[] = ASSET_KEYS): AssetRow[] {
  return keys.map((key) => {
    const asset = release ? findAsset(release, [assetNameForKey(release.tag_name, key)]) : undefined
    return asset
      ? { key, href: asset.browser_download_url, available: true, name: asset.name, size: formatSize(asset.size) }
      : { key, href: release?.html_url ?? RELEASES_URL, available: false }
  })
}
