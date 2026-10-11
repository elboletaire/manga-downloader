import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { assetOS, sumDownloads, compact, fetchStats, renderStats } from './stats'

describe('assetOS', () => {
  it('returns null for .md5 files', () => {
    expect(assetOS('manga-downloader-v1.14.0-darwin-arm64.tar.gz.md5')).toBeNull()
  })

  it('returns null for .sha256 files', () => {
    expect(assetOS('manga-downloader-v1.14.0-windows-amd64.zip.sha256')).toBeNull()
  })

  it('returns null for checksums files', () => {
    expect(assetOS('checksums.txt')).toBeNull()
  })

  it('returns windows for -windows- in name', () => {
    expect(assetOS('manga-downloader-v1.14.0-windows-amd64.zip')).toBe('windows')
    expect(assetOS('manga-downloader-gui-v1.14.0-windows-amd64.zip')).toBe('windows')
    expect(assetOS('manga-downloader-v1.14.0-windows-32bit-legacy.zip')).toBe('windows')
  })

  it('returns macos for -darwin- in name', () => {
    expect(assetOS('manga-downloader-v1.14.0-darwin-arm64.tar.gz')).toBe('macos')
    expect(assetOS('manga-downloader-gui-v1.14.0-darwin-universal.zip')).toBe('macos')
  })

  it('returns macos for -macos- in name', () => {
    expect(assetOS('manga-downloader-v1.14.0-macos-arm64.tar.gz')).toBe('macos')
  })

  it('returns linux for -linux- in name', () => {
    expect(assetOS('manga-downloader-v1.14.0-linux-amd64.tar.gz')).toBe('linux')
    expect(assetOS('manga-downloader-gui-v1.14.0-linux-arm64.tar.gz')).toBe('linux')
  })

  it('returns null for unrecognized names', () => {
    expect(assetOS('unknown-file.txt')).toBeNull()
    expect(assetOS('CHANGELOG.md')).toBeNull()
  })
})

describe('sumDownloads', () => {
  it('sums downloads by OS', () => {
    const releases = [
      {
        assets: [
          { name: 'manga-downloader-v1.14.0-windows-amd64.zip', download_count: 100 },
          { name: 'manga-downloader-v1.14.0-darwin-arm64.tar.gz', download_count: 50 },
          { name: 'manga-downloader-v1.14.0-linux-amd64.tar.gz', download_count: 75 },
          { name: 'manga-downloader-v1.14.0-windows-amd64.zip.md5', download_count: 5 }, // should be ignored
        ],
      },
    ]

    const result = sumDownloads(releases)

    expect(result.byOS.windows).toBe(100)
    expect(result.byOS.macos).toBe(50)
    expect(result.byOS.linux).toBe(75)
    expect(result.total).toBe(225)
  })

  it('ignores .md5 files', () => {
    const releases = [
      {
        assets: [
          { name: 'manga-downloader-v1.14.0-windows-amd64.zip', download_count: 100 },
          { name: 'manga-downloader-v1.14.0-windows-amd64.zip.md5', download_count: 1000 },
        ],
      },
    ]

    const result = sumDownloads(releases)

    expect(result.byOS.windows).toBe(100)
    expect(result.total).toBe(100)
  })

  it('handles empty releases', () => {
    const result = sumDownloads([])

    expect(result.byOS).toEqual({ windows: 0, macos: 0, linux: 0 })
    expect(result.total).toBe(0)
  })
})

describe('compact', () => {
  it('returns numbers < 100 as-is', () => {
    expect(compact(0)).toBe('0')
    expect(compact(18)).toBe('18')
    expect(compact(50)).toBe('50')
    expect(compact(99)).toBe('99')
  })

  it('rounds down to two significant digits and adds + when rounded off', () => {
    expect(compact(950)).toBe('950') // no rounding
    expect(compact(1000)).toBe('1K') // no rounding
    expect(compact(6146)).toBe('6.1K+') // 6146 → 6100, rounded off 46
    expect(compact(13319)).toBe('13K+') // 13319 → 13000, rounded off 319
    expect(compact(25065)).toBe('25K+') // 25065 → 25000, rounded off 65
  })

  it('handles large numbers', () => {
    expect(compact(1000000)).toBe('1M')
    expect(compact(1234567)).toBe('1.2M+')
  })
})

describe('fetchStats', () => {
  let originalEnv: string | undefined

  beforeEach(() => {
    originalEnv = process.env.STATS
  })

  afterEach(() => {
    if (originalEnv === undefined) {
      delete process.env.STATS
    } else {
      process.env.STATS = originalEnv
    }
  })

  it('returns null when STATS=off', async () => {
    process.env.STATS = 'off'
    const result = await fetchStats(vi.fn())
    expect(result).toBeNull()
  })

  it('paginates through GitHub API', async () => {
    const mockFetch = vi.fn()

    // First page: 100 releases (continues pagination)
    const page1: any = Array(100).fill(null).map(() => ({ assets: [{ name: 'test-windows-amd64.zip', download_count: 1 }] }))
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: vi.fn(async () => page1),
    })

    // Second page: 50 releases (fewer than 100, so pagination stops)
    const page2: any = Array(50).fill(null).map(() => ({ assets: [{ name: 'test-windows-amd64.zip', download_count: 1 }] }))
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: vi.fn(async () => page2),
    })

    // Docker Hub call
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: vi.fn(async () => ({ pull_count: 1000 })),
    })

    const result = await fetchStats(mockFetch)

    expect(result).not.toBeNull()
    expect(result?.total).toBe(150)
    expect(result?.dockerPulls).toBe(1000)
    expect(mockFetch).toHaveBeenCalledTimes(3)
  })

  it('continues pagination until page returns fewer than 100', async () => {
    const mockFetch = vi.fn()

    // First page: 100 releases (continues pagination)
    const page1: any = Array(100).fill(null).map(() => ({ assets: [{ name: 'test-windows-amd64.zip', download_count: 1 }] }))
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: vi.fn(async () => page1),
    })

    // Second page: 50 releases (fewer than 100, stops pagination)
    const page2: any = Array(50).fill(null).map(() => ({ assets: [{ name: 'test-windows-amd64.zip', download_count: 1 }] }))
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: vi.fn(async () => page2),
    })

    // Docker Hub call
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: vi.fn(async () => ({ pull_count: 500 })),
    })

    const result = await fetchStats(mockFetch)

    expect(result).not.toBeNull()
    expect(result?.total).toBe(150)
    expect(mockFetch).toHaveBeenCalledTimes(3)
  })

  it('returns null on GitHub failure', async () => {
    const mockFetch = vi.fn()
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500,
    })

    const result = await fetchStats(mockFetch)

    expect(result).toBeNull()
  })

  it('returns GitHub numbers with null dockerPulls on Docker failure', async () => {
    const mockFetch = vi.fn()

    // GitHub call
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => [
        { assets: [{ name: 'manga-downloader-v1.0.0-windows-amd64.zip', download_count: 100 }] },
      ],
    })

    // Empty page (pagination ends)
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => [],
    })

    // Docker Hub call fails
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 404,
    })

    const result = await fetchStats(mockFetch)

    expect(result).not.toBeNull()
    expect(result?.total).toBe(100)
    expect(result?.dockerPulls).toBeNull()
  })
})

describe('renderStats', () => {
  it('returns empty strings for all placeholders when stats is null', () => {
    const result = renderStats(null)

    expect(result['stats:hero']).toBe('')
    expect(result['stats:windows']).toBe('')
    expect(result['stats:macos']).toBe('')
    expect(result['stats:linux']).toBe('')
  })

  it('renders hero stats with both downloads and docker pulls', () => {
    const stats = {
      total: 13319,
      byOS: { windows: 6146, macos: 4000, linux: 3173 },
      dockerPulls: 25065,
    }

    const result = renderStats(stats)

    expect(result['stats:hero']).toContain('<div class="hero-stats" data-stats>')
    expect(result['stats:hero']).toContain('<strong>13K+</strong>')
    expect(result['stats:hero']).toContain('<span>downloads</span>')
    expect(result['stats:hero']).toContain('<strong>25K+</strong>')
    expect(result['stats:hero']).toContain('<span>Docker pulls</span>')
  })

  it('renders hero stats without docker pulls when null', () => {
    const stats = {
      total: 13319,
      byOS: { windows: 6146, macos: 4000, linux: 3173 },
      dockerPulls: null,
    }

    const result = renderStats(stats)

    expect(result['stats:hero']).toContain('<strong>13K+</strong>')
    expect(result['stats:hero']).toContain('<span>downloads</span>')
    expect(result['stats:hero']).not.toContain('Docker pulls')
  })

  it('renders platform stats for each OS', () => {
    const stats = {
      total: 13319,
      byOS: { windows: 6146, macos: 4000, linux: 3173 },
      dockerPulls: null,
    }

    const result = renderStats(stats)

    expect(result['stats:windows']).toBe('<p class="platform-stats"><strong>6.1K+</strong> downloads</p>')
    expect(result['stats:macos']).toBe('<p class="platform-stats"><strong>4K</strong> downloads</p>')
    expect(result['stats:linux']).toBe('<p class="platform-stats"><strong>3.1K+</strong> downloads</p>')
  })

  it('returns empty string for OS with 0 count', () => {
    const stats = {
      total: 5000,
      byOS: { windows: 5000, macos: 0, linux: 0 },
      dockerPulls: null,
    }

    const result = renderStats(stats)

    expect(result['stats:windows']).toContain('<strong>5K</strong>')
    expect(result['stats:macos']).toBe('')
    expect(result['stats:linux']).toBe('')
  })
})
