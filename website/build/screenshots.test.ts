import { describe, expect, it } from 'vitest'
import { localizeScreenshots } from './screenshots'

const html = `<source srcset="/src/assets/screenshots/app-overview-dark-1000.webp 1000w, /src/assets/screenshots/app-overview-dark-1600.webp 1600w" />
<img src="/src/assets/screenshots/volumes-light-1000.webp" srcset="/src/assets/screenshots/volumes-light-600.webp 600w" />
<meta property="og:image" content="/opengraph.png" />`

describe('localizeScreenshots', () => {
  it('points every screenshot at the language folder when its WebP exists', () => {
    const out = localizeScreenshots(html, 'es', () => true)
    expect(out).toContain('srcset="/src/assets/screenshots/es/app-overview-dark-1000.webp 1000w, /src/assets/screenshots/es/app-overview-dark-1600.webp 1600w"')
    expect(out).toContain('src="/src/assets/screenshots/es/volumes-light-1000.webp"')
    expect(out).toContain('srcset="/src/assets/screenshots/es/volumes-light-600.webp 600w"')
  })

  it('falls back to the English file per image when the translated one is missing', () => {
    const out = localizeScreenshots(html, 'ca', (url) => url.endsWith('/ca/volumes-light-1000.webp'))
    expect(out).toContain('src="/src/assets/screenshots/ca/volumes-light-1000.webp"')
    expect(out).toContain('srcset="/src/assets/screenshots/volumes-light-600.webp 600w"')
    expect(out).toContain('/src/assets/screenshots/app-overview-dark-1000.webp 1000w')
  })

  it('checks the file for the language it rewrites to', () => {
    const asked: string[] = []
    localizeScreenshots(html, 'es', (url) => (asked.push(url), false))
    expect(asked).toContain('/src/assets/screenshots/es/app-overview-dark-1600.webp')
    expect(asked.every((url) => url.startsWith('/src/assets/screenshots/es/'))).toBe(true)
  })

  it('leaves the English page untouched, even when the files exist', () => {
    expect(localizeScreenshots(html, 'en', () => true)).toBe(html)
  })

  it('leaves other assets and already-localized URLs alone', () => {
    const other = '<img src="/src/assets/brand/logo.svg" /><img src="/src/assets/screenshots/es/x.webp" />'
    expect(localizeScreenshots(other, 'es', () => true)).toBe(other)
  })
})
