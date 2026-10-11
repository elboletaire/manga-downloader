import type { Release as PageRelease } from '../src/release.ts'

export type OSKey = 'windows' | 'macos' | 'linux'

export interface Stats {
  total: number
  byOS: Record<OSKey, number>
  dockerPulls: number | null
  latest: PageRelease | null
}

interface Asset {
  name: string
  download_count: number
}

interface Release {
  tag_name: string
  html_url?: string
  draft?: boolean
  prerelease?: boolean
  assets: (Asset & { browser_download_url?: string; size?: number })[]
}

/**
 * Determine the OS from an asset name.
 * - Names containing `.md5`, `.sha256`, `checksums` → null
 * - Names containing `-windows-` → windows
 * - Names containing `-darwin-` or `-macos-` → macos
 * - Names containing `-linux-` → linux
 * - Otherwise → null
 */
export function assetOS(name: string): OSKey | null {
  if (/\.(md5|sha256)$/.test(name) || name.includes('checksums')) {
    return null
  }
  if (name.includes('-windows-')) {
    return 'windows'
  }
  if (name.includes('-darwin-') || name.includes('-macos-')) {
    return 'macos'
  }
  if (name.includes('-linux-')) {
    return 'linux'
  }
  return null
}

/**
 * Find the latest release that is neither draft nor prerelease.
 * Returns the first such release (API lists newest first) trimmed to PageRelease shape,
 * or null if none found. Matches what `/releases/latest` returns.
 */
export function latestRelease(releases: Release[]): PageRelease | null {
  for (const release of releases) {
    if (release.draft || release.prerelease) {
      continue
    }
    return {
      tag_name: release.tag_name,
      html_url: release.html_url,
      assets: release.assets.map((a) => ({
        name: a.name,
        browser_download_url: a.browser_download_url ?? '',
        size: a.size,
      })),
    }
  }
  return null
}

/**
 * Sum downloads from releases, grouping by OS.
 * Only assets whose assetOS() isn't null are counted.
 * total is the sum of the three OSes.
 */
export function sumDownloads(releases: Release[]): { total: number; byOS: Record<OSKey, number> } {
  const byOS: Record<OSKey, number> = { windows: 0, macos: 0, linux: 0 }
  let total = 0

  for (const release of releases) {
    for (const asset of release.assets) {
      const os = assetOS(asset.name)
      if (os) {
        byOS[os] += asset.download_count
        total += asset.download_count
      }
    }
  }

  return { total, byOS }
}

/**
 * Round down to two significant digits and format with compact notation.
 * Add `+` when rounding off anything.
 *
 * Examples:
 * - 13319 → "13K+"
 * - 6146 → "6.1K+"
 * - 25065 → "25K+"
 * - 1000 → "1K"
 * - 950 → "950"
 * - 0 → "0"
 * - 18 → "18"
 */
export function compact(n: number): string {
  if (n < 100) {
    return String(n)
  }

  // Calculate number of digits
  const digits = Math.floor(Math.log10(n)) + 1

  // Round down to two significant digits
  const rounded = Math.floor(n / 10 ** (digits - 2)) * 10 ** (digits - 2)

  // Format with compact notation
  const formatter = new Intl.NumberFormat('en', {
    notation: 'compact',
    maximumSignificantDigits: 2,
  })

  const formatted = formatter.format(rounded)

  // Add + if we rounded anything off
  const hasRoundedOff = rounded < n
  return hasRoundedOff ? formatted + '+' : formatted
}

/**
 * Fetch stats from GitHub releases API and Docker Hub.
 * Returns null if GitHub fails (to avoid breaking the build).
 * Docker failure returns null for dockerPulls but GitHub numbers still show.
 * Each request has a 10s timeout.
 * STATS=off env var skips fetching and returns null.
 */
export async function fetchStats(fetchFn: typeof fetch = fetch): Promise<Stats | null> {
  if (process.env.STATS === 'off') {
    return null
  }

  try {
    const releases = await fetchAllReleases(fetchFn)
    const { total, byOS } = sumDownloads(releases)
    const dockerPulls = await fetchDockerPulls(fetchFn)
    const latest = latestRelease(releases)

    return { total, byOS, dockerPulls, latest }
  } catch (error) {
    console.warn(`Failed to fetch stats: ${error instanceof Error ? error.message : String(error)}`)
    return null
  }
}

async function fetchAllReleases(fetchFn: typeof fetch): Promise<Release[]> {
  const releases: Release[] = []
  let page = 1

  while (true) {
    const url = `https://api.github.com/repos/elboletaire/manga-downloader/releases?per_page=100&page=${page}`
    const headers: Record<string, string> = {
      Accept: 'application/vnd.github+json',
      'User-Agent': 'manga-downloader-website',
    }

    if (process.env.GITHUB_TOKEN) {
      headers.Authorization = `Bearer ${process.env.GITHUB_TOKEN}`
    }

    const response = await fetchFn(url, { headers, signal: AbortSignal.timeout(10000) })

    if (!response.ok) {
      throw new Error(`GitHub API returned ${response.status}`)
    }

    const data = await response.json()

    if (!Array.isArray(data)) {
      throw new Error('GitHub API response is not an array')
    }

    releases.push(...data)

    if (data.length < 100) {
      break
    }

    page++
  }

  return releases
}

async function fetchDockerPulls(fetchFn: typeof fetch): Promise<number | null> {
  const url = 'https://hub.docker.com/v2/repositories/elboletaire/manga-downloader/'

  try {
    const response = await fetchFn(url, { signal: AbortSignal.timeout(10000) })

    if (!response.ok) {
      console.warn(`Failed to fetch Docker Hub stats: ${response.status}`)
      return null
    }

    const data = await response.json()

    if (typeof data?.pull_count !== 'number') {
      console.warn('Docker Hub response missing pull_count')
      return null
    }

    return data.pull_count
  } catch (error) {
    console.warn(`Failed to fetch Docker Hub stats: ${error instanceof Error ? error.message : String(error)}`)
    return null
  }
}

/**
 * Render stats to HTML strings for template replacement.
 * Returns a record mapping placeholder names to HTML strings.
 */
export interface StatsLabels { downloads: string; dockerPulls: string }

export const EN_LABELS: StatsLabels = { downloads: 'downloads', dockerPulls: 'Docker pulls' }

export function renderStats(stats: Stats | null, labels: StatsLabels = EN_LABELS): Record<string, string> {
  const result: Record<string, string> = {}

  if (!stats) {
    result['stats:hero'] = ''
    result['stats:windows'] = ''
    result['stats:macos'] = ''
    result['stats:linux'] = ''
    return result
  }

  // Hero stats: total downloads and docker pulls
  let heroHtml = `<div class="hero-stats" data-stats>`
  heroHtml += `<div><strong>${compact(stats.total)}</strong><span>${labels.downloads}</span></div>`

  if (stats.dockerPulls !== null) {
    heroHtml += `<div><strong>${compact(stats.dockerPulls)}</strong><span>${labels.dockerPulls}</span></div>`
  }

  heroHtml += `</div>`
  result['stats:hero'] = heroHtml

  // Platform stats
  const platforms: OSKey[] = ['windows', 'macos', 'linux']
  for (const os of platforms) {
    const count = stats.byOS[os]
    if (count > 0) {
      result[`stats:${os}`] = `<p class="platform-stats"><strong>${compact(count)}</strong> ${labels.downloads}</p>`
    } else {
      result[`stats:${os}`] = ''
    }
  }

  return result
}
