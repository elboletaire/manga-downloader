import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { type Site, parseSites, parseVerified, renderSites, EN_BADGE, EN_VERIFIED } from './sites'

const sample = `
## Features

- [not a site](https://example.com)

## Supported sites

<details>
<summary><b>Show all 3 supported sites</b></summary>

| Site | Notes | Last verified |
| --- | --- | --- |
| [asurascans.com](https://asurascans.com) | Asura Scans, former asuratoon.com | 2026-10-15 |
| [comix.to](https://comix.to) \\* |  | — |
| [baozimh.com](https://www.baozimh.com) | 包子漫画 | ❌ failing since 2026-10-01 |

Verified weekly by CI.

</details>

Other sites [open an issue](https://github.com/x/issues).

## Installation

- [also not a site](https://example.org)
`

describe('parseSites', () => {
  it('reads only the supported sites section', () => {
    expect(parseSites(sample)).toEqual([
      { domain: 'asurascans.com', note: 'Asura Scans, former asuratoon.com', url: 'https://asurascans.com', browser: false, verified: { status: 'ok', date: '2026-10-15' } },
      { domain: 'comix.to', note: '', url: 'https://comix.to', browser: true, verified: { status: 'unknown' } },
      { domain: 'baozimh.com', note: '包子漫画', url: 'https://www.baozimh.com', browser: false, verified: { status: 'failing', date: '2026-10-01' } },
    ])
  })

  it('fails loudly when the section is missing or empty', () => {
    expect(() => parseSites('# nothing here')).toThrow(/Supported sites/)
    expect(() => parseSites('## Supported sites\n\nnone yet\n')).toThrow(/no sites/)
  })

  it('parses the real README', () => {
    const readme = readFileSync(fileURLToPath(new URL('../../README.md', import.meta.url)), 'utf8')
    const sites = parseSites(readme)
    // The summary line states the count; keep both in agreement.
    const stated = Number(/Show all (\d+) supported sites/.exec(readme)?.[1])
    expect(sites.length).toBe(stated)
    expect(sites.some((s) => s.browser)).toBe(true)
    expect(sites.find((s) => s.domain === 'mangadex.org')).toMatchObject({ note: 'MangaDex', browser: false })
  })
})

describe('renderSites', () => {
  it('escapes and marks browser sites', () => {
    const html = renderSites([{ domain: 'a<b>.com', note: 'x & y', url: 'https://a.com/?q="1"', browser: true, verified: { status: 'unknown' } }])
    expect(html).toContain('a&lt;b&gt;.com')
    expect(html).toContain('x &amp; y')
    expect(html).toContain('href="https://a.com/?q=&quot;1&quot;"')
    expect(html).toContain('data-browser')
    expect(html).toContain('<span class="visually-hidden">needs a </span>browser</span>')
  })
})

describe('parseVerified', () => {
  it('reads dates, failures and placeholders', () => {
    expect(parseVerified(' 2026-10-15 ')).toEqual({ status: 'ok', date: '2026-10-15' })
    expect(parseVerified('❌ failing since 2026-10-15')).toEqual({ status: 'failing', date: '2026-10-15' })
    expect(parseVerified('—')).toEqual({ status: 'unknown' })
  })
})

describe('renderSites verified', () => {
  const base = { domain: 'a.com', note: '', url: 'https://a.com', browser: false }
  const line = (verified: Site['verified'], locale?: string) => renderSites([{ ...base, verified }], undefined, undefined, locale)
  it('shows a visible, dated line with a machine-readable time', () => {
    const ok = line({ status: 'ok', date: '2026-10-15' })
    expect(ok).toContain('data-verified="ok"')
    expect(ok).toContain('Verified <time datetime="2026-10-15">Oct 15, 2026</time>')
    expect(ok).not.toContain('title=')
    expect(line({ status: 'failing', date: '2026-10-01' })).toContain('Failing since <time datetime="2026-10-01">Oct 1, 2026</time>')
  })
  it('says "not checked" without a date for unknown', () => {
    const html = line({ status: 'unknown' })
    expect(html).toContain('data-verified="unknown"')
    expect(html).toContain('Not checked')
    expect(html).not.toContain('<time')
  })
  it('refuses a label without {{date}}', () => {
    const bad = { on: 'Verified', failing: 'Failing', unknown: 'Not checked' }
    expect(() => renderSites([{ ...base, verified: { status: 'ok', date: '2026-10-15' } }], undefined, bad)).toThrow(/\{\{date\}\}/)
  })
  it('localizes the date and keeps it out of the filter text', () => {
    expect(line({ status: 'ok', date: '2026-10-15' }, 'es')).toContain('15 oct 2026')
    expect(line({ status: 'ok', date: '2026-10-15' })).toContain('data-search="a.com "')
  })
})

describe('EN_BADGE', () => {
  it('matches en.json sites.badge labels', () => {
    const enJson = JSON.parse(readFileSync(fileURLToPath(new URL('../locales/en.json', import.meta.url)), 'utf8'))
    expect(EN_BADGE.label).toBe(enJson.sites.badge.label)
    expect(EN_BADGE.srPrefix).toBe(enJson.sites.badge.srPrefix)
    expect(EN_BADGE.title).toBe(enJson.sites.badge.title)
  })
})

describe('EN_VERIFIED', () => {
  it('matches en.json sites.verified labels', () => {
    const enJson = JSON.parse(readFileSync(fileURLToPath(new URL('../locales/en.json', import.meta.url)), 'utf8'))
    expect(EN_VERIFIED).toEqual(enJson.sites.verified)
  })
})
