import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { parseSites, renderSites, EN_BADGE } from './sites'

const sample = `
## Features

- [not a site](https://example.com)

## Supported sites

<details>
<summary><b>Show all 3 supported sites</b></summary>

- [asurascans.com (Asura Scans, former asuratoon.com)](https://asurascans.com)
- [comix.to](https://comix.to) \\*
- [baozimh.com (包子漫画)](https://www.baozimh.com)

</details>

Other sites [open an issue](https://github.com/x/issues).

## Installation

- [also not a site](https://example.org)
`

describe('parseSites', () => {
  it('reads only the supported sites section', () => {
    expect(parseSites(sample)).toEqual([
      { domain: 'asurascans.com', note: 'Asura Scans, former asuratoon.com', url: 'https://asurascans.com', browser: false },
      { domain: 'comix.to', note: '', url: 'https://comix.to', browser: true },
      { domain: 'baozimh.com', note: '包子漫画', url: 'https://www.baozimh.com', browser: false },
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
    const html = renderSites([{ domain: 'a<b>.com', note: 'x & y', url: 'https://a.com/?q="1"', browser: true }])
    expect(html).toContain('a&lt;b&gt;.com')
    expect(html).toContain('x &amp; y')
    expect(html).toContain('href="https://a.com/?q=&quot;1&quot;"')
    expect(html).toContain('data-browser')
    expect(html).toContain('<span class="visually-hidden">needs a </span>browser</span>')
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
