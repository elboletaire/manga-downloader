// Pure logic behind the Download button: which OS the visitor is on, and
// which release asset matches it. No DOM access here, so it's all testable.

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
      // ARM Windows runs the x64 build emulated if there's no native one.
      return p.arch === 'arm64'
        ? [`${base}-windows-arm64.zip`, `${base}-windows-amd64.zip`]
        : [`${base}-windows-amd64.zip`]
    case 'macos':
      return [`${base}-darwin-universal.zip`]
    case 'linux':
      return [`${base}-linux-${p.arch === 'arm64' ? 'arm64' : 'amd64'}.tar.gz`]
    default:
      return []
  }
}

/** Command line asset names for a platform, in order of preference. */
export function cliAssetNames(tag: string, p: Platform): string[] {
  const base = `manga-downloader-${tag}`
  switch (p.os) {
    case 'windows':
      if (p.arch === 'arm64') return [`${base}-windows-arm64.zip`, `${base}-windows-amd64.zip`]
      if (p.arch === '386') return [`${base}-windows-32bit-legacy.zip`, `${base}-windows-386.zip`]
      return [`${base}-windows-amd64.zip`]
    case 'macos':
      // Browsers rarely say which Mac they run on, and Apple silicon is the
      // safe default: Intel Macs are the ones that have to ask.
      return [`${base}-darwin-${p.arch === 'amd64' ? 'amd64' : 'arm64'}.tar.gz`]
    case 'linux': {
      const arch = p.arch === 'unknown' ? 'amd64' : p.arch
      return [`${base}-linux-${arch}.tar.gz`]
    }
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
  if (/darwin-arm64/.test(assetName)) return 'Apple silicon'
  if (/darwin-amd64/.test(assetName)) return 'Intel'
  if (/arm64/.test(assetName)) return 'ARM64'
  if (/amd64/.test(assetName)) return 'x64'
  if (/32bit|386/.test(assetName)) return '32-bit'
  return ''
}

export function formatSize(bytes: number | undefined): string {
  if (!bytes) return ''
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

export interface DownloadChoice {
  /** `gui`/`cli` link straight to a file; `page` to the releases page. */
  kind: 'gui' | 'cli' | 'page'
  href: string
  label: string
  detail: string
}

/**
 * Picks what the hero's Download button offers. `release` is null when the
 * GitHub API couldn't be reached. When the desktop app isn't published yet,
 * the button offers the command line build instead and says so.
 */
export function chooseDownload(release: Release | null, p: Platform): DownloadChoice {
  if (p.os === 'unknown') {
    return {
      kind: 'page',
      href: RELEASES_URL,
      label: 'Download',
      detail: 'For Windows, macOS and Linux',
    }
  }
  const osName = OS_NAMES[p.os]
  if (!release) {
    return {
      kind: 'page',
      href: RELEASES_URL,
      label: `Download for ${osName}`,
      detail: 'Pick your file on the releases page',
    }
  }

  const tag = release.tag_name
  const gui = findAsset(release, guiAssetNames(tag, p))
  if (gui) {
    return {
      kind: 'gui',
      href: gui.browser_download_url,
      label: `Download for ${osName}`,
      detail: joinDetail([`Desktop app ${tag}`, archLabel(gui.name), formatSize(gui.size)]),
    }
  }

  const cli = findAsset(release, cliAssetNames(tag, p))
  if (cli) {
    return {
      kind: 'cli',
      href: cli.browser_download_url,
      label: `Get the CLI for ${osName}`,
      detail: joinDetail([`Command line ${tag}`, archLabel(cli.name), formatSize(cli.size)]),
    }
  }

  return {
    kind: 'page',
    href: release.html_url ?? RELEASES_URL,
    label: `Download for ${osName}`,
    detail: `Pick your file from ${tag} on GitHub`,
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
