import { describe, expect, it } from 'vitest'
import {
  RELEASES_URL,
  archLabel,
  chooseDownload,
  detectPlatform,
  guiAssetNames,
  resolveAssets,
  type Release,
} from './release'

const UA = {
  win: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36',
  mac: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15',
  linux: 'Mozilla/5.0 (X11; Linux x86_64; rv:131.0) Gecko/20100101 Firefox/131.0',
  linuxArm: 'Mozilla/5.0 (X11; Linux aarch64; rv:131.0) Gecko/20100101 Firefox/131.0',
  android: 'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Mobile Safari/537.36',
  iphone: 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1',
  cros: 'Mozilla/5.0 (X11; CrOS x86_64 14541.0.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36',
}

describe('detectPlatform', () => {
  it.each([
    [UA.win, 'windows', 'amd64'],
    [UA.mac, 'macos', 'unknown'],
    [UA.linux, 'linux', 'amd64'],
    [UA.linuxArm, 'linux', 'arm64'],
    [UA.android, 'unknown', 'unknown'],
    [UA.iphone, 'unknown', 'unknown'],
    [UA.cros, 'unknown', 'amd64'],
    ['', 'unknown', 'unknown'],
  ])('reads the user agent %#', (userAgent, os, arch) => {
    expect(detectPlatform({ userAgent })).toEqual({ os, arch })
  })

  it('prefers userAgentData over the user agent string', () => {
    expect(detectPlatform({ userAgent: UA.win, uaPlatform: 'macOS' }).os).toBe('macos')
    expect(detectPlatform({ userAgent: UA.win, uaPlatform: 'Android' }).os).toBe('unknown')
  })

  it('uses the high-entropy architecture hints', () => {
    expect(detectPlatform({ userAgent: UA.win, uaPlatform: 'Windows', architecture: 'arm', bitness: '64' }))
      .toEqual({ os: 'windows', arch: 'arm64' })
    expect(detectPlatform({ userAgent: UA.win, architecture: 'x86', bitness: '32' }).arch).toBe('386')
  })
})

describe('asset names', () => {
  const tag = 'v1.11.0'

  it('names the desktop app archives', () => {
    expect(guiAssetNames(tag, { os: 'windows', arch: 'amd64' })).toEqual([
      'manga-downloader-gui-v1.11.0-windows-amd64-installer.exe',
      'manga-downloader-gui-v1.11.0-windows-amd64.zip',
    ])
    expect(guiAssetNames(tag, { os: 'windows', arch: 'arm64' })).toEqual([
      'manga-downloader-gui-v1.11.0-windows-arm64-installer.exe',
      'manga-downloader-gui-v1.11.0-windows-amd64-installer.exe',
      'manga-downloader-gui-v1.11.0-windows-arm64.zip',
      'manga-downloader-gui-v1.11.0-windows-amd64.zip',
    ])
    expect(guiAssetNames(tag, { os: 'macos', arch: 'unknown' })).toEqual(['manga-downloader-gui-v1.11.0-darwin-universal.zip'])
    expect(guiAssetNames(tag, { os: 'linux', arch: 'unknown' })).toEqual(['manga-downloader-gui-v1.11.0-linux-amd64.tar.gz'])
    expect(guiAssetNames(tag, { os: 'linux', arch: 'arm64' })).toEqual(['manga-downloader-gui-v1.11.0-linux-arm64.tar.gz'])
    expect(guiAssetNames(tag, { os: 'unknown', arch: 'unknown' })).toEqual([])
  })

  it('labels architectures', () => {
    expect(archLabel('manga-downloader-gui-v1.11.0-darwin-universal.zip')).toBe('Universal')
    expect(archLabel('manga-downloader-v1.11.0-windows-arm64.zip')).toBe('ARM64')
    expect(archLabel('manga-downloader-v1.11.0-windows-amd64.zip')).toBe('x64')
  })
})

const asset = (name: string, size = 10 * 1024 * 1024) => ({
  name,
  size,
  browser_download_url: `https://github.com/elboletaire/manga-downloader/releases/download/v/${name}`,
})

const withGUI: Release = {
  tag_name: 'v1.11.0',
  html_url: 'https://github.com/elboletaire/manga-downloader/releases/tag/v1.11.0',
  assets: [
    asset('manga-downloader-gui-v1.11.0-windows-amd64.zip'),
    asset('manga-downloader-gui-v1.11.0-darwin-universal.zip'),
    asset('manga-downloader-gui-v1.11.0-linux-amd64.tar.gz'),
    asset('manga-downloader-v1.11.0-windows-amd64.zip'),
    asset('manga-downloader-v1.11.0-linux-amd64.tar.gz'),
    asset('manga-downloader-v1.11.0-linux-amd64.tar.gz.md5', 40),
  ],
}

const cliOnly: Release = {
  tag_name: 'v1.10.0',
  html_url: 'https://github.com/elboletaire/manga-downloader/releases/tag/v1.10.0',
  assets: [
    asset('manga-downloader-v1.10.0-windows-amd64.zip'),
    asset('manga-downloader-v1.10.0-darwin-arm64.tar.gz'),
    asset('manga-downloader-v1.10.0-linux-amd64.tar.gz'),
  ],
}

describe('chooseDownload', () => {
  const withAllGUI: Release = {
    ...withGUI,
    assets: [
      asset('manga-downloader-gui-v1.11.0-windows-amd64.zip'),
      asset('manga-downloader-gui-v1.11.0-windows-arm64.zip'),
      asset('manga-downloader-gui-v1.11.0-darwin-universal.zip'),
      asset('manga-downloader-gui-v1.11.0-linux-amd64.tar.gz'),
      asset('manga-downloader-gui-v1.11.0-linux-arm64.tar.gz'),
    ],
  }

  it.each([
    [{ os: 'windows', arch: 'amd64' }, 'windows-amd64.zip', 'Download for Windows'],
    [{ os: 'windows', arch: 'arm64' }, 'windows-arm64.zip', 'Download for Windows'],
    [{ os: 'macos', arch: 'unknown' }, 'darwin-universal.zip', 'Download for macOS'],
    [{ os: 'linux', arch: 'amd64' }, 'linux-amd64.tar.gz', 'Download for Linux'],
    [{ os: 'linux', arch: 'arm64' }, 'linux-arm64.tar.gz', 'Download for Linux'],
  ] as const)('offers the desktop app for %j', (platform, file, label) => {
    const c = chooseDownload(withAllGUI, platform)
    expect(c.kind).toBe('gui')
    expect(c.href.endsWith(`/manga-downloader-gui-v1.11.0-${file}`)).toBe(true)
    expect(c.label).toBe(label)
    expect(c.detail).toMatch(/^Desktop app v1\.11\.0 · /)
  })

  it('prefers the Windows installer over the zip', () => {
    const r: Release = {
      ...withGUI,
      assets: [
        asset('manga-downloader-gui-v1.11.0-windows-amd64.zip'),
        asset('manga-downloader-gui-v1.11.0-windows-amd64-installer.exe'),
      ],
    }
    const c = chooseDownload(r, { os: 'windows', arch: 'amd64' })
    expect(c.href).toMatch(/windows-amd64-installer\.exe$/)
    expect(c.detail).toMatch(/x64/)
  })

  it('falls back to the x64 desktop app on ARM Windows', () => {
    const c = chooseDownload(withGUI, { os: 'windows', arch: 'arm64' })
    expect(c.kind).toBe('gui')
    expect(c.href).toMatch(/gui-v1\.11\.0-windows-amd64\.zip$/)
  })

  it('links to the releases page when the desktop app file is missing', () => {
    const c = chooseDownload(cliOnly, { os: 'macos', arch: 'unknown' })
    expect(c).toEqual({
      kind: 'page',
      href: RELEASES_URL,
      label: 'Download for macOS',
      detail: 'Latest release on GitHub',
    })
  })

  it('links to the releases page when the API fails', () => {
    const c = chooseDownload(null, { os: 'linux', arch: 'amd64' })
    expect(c).toEqual({
      kind: 'page',
      href: RELEASES_URL,
      label: 'Download for Linux',
      detail: 'Latest release on GitHub',
    })
  })

  it('keeps a generic desktop app button for unsupported systems', () => {
    const generic = { kind: 'page', href: '#downloads', label: 'Download the desktop app' }
    expect(chooseDownload(withGUI, { os: 'unknown', arch: 'unknown' })).toMatchObject(generic)
    expect(chooseDownload(null, { os: 'unknown', arch: 'unknown' })).toMatchObject(generic)
  })
})

describe('resolveAssets', () => {
  it('links what exists and points the rest at the release page', () => {
    const rows = resolveAssets(cliOnly, ['gui-windows-amd64.zip', 'windows-amd64.zip'])
    expect(rows[0]).toMatchObject({ key: 'gui-windows-amd64.zip', available: false, href: cliOnly.html_url })
    expect(rows[1]).toMatchObject({ available: true, name: 'manga-downloader-v1.10.0-windows-amd64.zip', size: '10.0 MB' })
  })

  it('falls back to the releases page without a release', () => {
    expect(resolveAssets(null, ['linux-amd64.tar.gz'])[0]).toEqual({
      key: 'linux-amd64.tar.gz',
      href: RELEASES_URL,
      available: false,
    })
  })
})
