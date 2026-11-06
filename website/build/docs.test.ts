import { describe, expect, it } from 'vitest'
import { buildManual, MANUALS, REPO, renderMarkdown, rewriteLinks, rewriteTarget, slugify } from './docs'

const options = {
  base: '/',
  siteURL: 'https://mangadownloader.cat/',
  stylesheet: '/assets/main.css',
  brand: { light: '/assets/lockup.svg', dark: '/assets/lockup-dark.svg' },
}

describe('slugify', () => {
  it('makes GitHub-style anchors', () => {
    expect(slugify('Environment variables')).toBe('environment-variables')
    expect(slugify('`--skip-pages`')).toBe('--skip-pages')
    expect(slugify("What's new?")).toBe('whats-new')
  })
})

describe('rewriteTarget', () => {
  it('sends links between manuals to their pages, keeping the anchor', () => {
    expect(rewriteTarget('cli.md#environment-variables', 'docs/app.md', '/')).toEqual({ url: '/docs/cli/#environment-variables' })
    expect(rewriteTarget('app.md', 'docs/cli.md', '/manga-downloader/')).toEqual({ url: '/manga-downloader/docs/app/' })
  })

  it('leaves in-page and external targets alone', () => {
    expect(rewriteTarget('#options', 'docs/cli.md', '/')).toEqual({ url: '#options' })
    expect(rewriteTarget('https://ko-fi.com/elboletaire', 'docs/app.md', '/')).toEqual({ url: 'https://ko-fi.com/elboletaire' })
  })

  it('sends repo files to GitHub', () => {
    expect(rewriteTarget('../README.md#installing-the-command-line-tool', 'docs/cli.md', '/')).toEqual({
      url: `${REPO}/blob/master/README.md#installing-the-command-line-tool`,
    })
  })

  it('serves repo assets from the copied media folder and reports them to copy', () => {
    expect(rewriteTarget('../assets/demos/prompt-light.gif', 'docs/cli.md', '/')).toEqual({
      url: '/docs/media/demos/prompt-light.gif',
      asset: 'assets/demos/prompt-light.gif',
    })
  })
})

describe('rewriteLinks', () => {
  it('rewrites href, src and srcset, and lists each asset once', () => {
    const html =
      '<p><a href="../README.md">readme</a> <a href="app.md">app</a></p>' +
      '<picture><source media="(prefers-color-scheme: dark)" srcset="../assets/screenshots/a-dark.webp" />' +
      '<img src="../assets/screenshots/a-light.webp" srcset="../assets/screenshots/a-light.webp 1x, ../assets/screenshots/a-light-2x.webp 2x" /></picture>'
    const out = rewriteLinks(html, 'docs/app.md', '/')
    expect(out.html).toContain(`href="${REPO}/blob/master/README.md"`)
    expect(out.html).toContain('href="/docs/app/"')
    expect(out.html).toContain('srcset="/docs/media/screenshots/a-dark.webp"')
    expect(out.html).toContain('src="/docs/media/screenshots/a-light.webp"')
    expect(out.html).toContain('srcset="/docs/media/screenshots/a-light.webp 1x, /docs/media/screenshots/a-light-2x.webp 2x"')
    expect(out.assets).toEqual(['assets/screenshots/a-dark.webp', 'assets/screenshots/a-light.webp', 'assets/screenshots/a-light-2x.webp'])
  })
})

describe('renderMarkdown', () => {
  it('gives headings anchors, numbering repeats, and renders GFM tables', () => {
    const html = renderMarkdown('# Guide\n\n## Options\n\n| a | b |\n| - | - |\n| 1 | 2 |\n\n## Options\n')
    expect(html).toContain('<h1 id="guide">Guide</h1>')
    expect(html).toContain('<h2 id="options">Options</h2>')
    expect(html).toContain('<h2 id="options-1">Options</h2>')
    expect(html).toContain('<table>')
  })

  it('slugs the rendered heading text, not its Markdown', () => {
    expect(renderMarkdown('## Use `--skip-pages` and [links](x.md)')).toContain('<h2 id="use---skip-pages-and-links">')
  })
})

describe('MANUALS', () => {
  it('keeps titles and descriptions within search snippet lengths', () => {
    for (const manual of MANUALS) {
      expect(manual.title.length).toBeLessThanOrEqual(60)
      expect(manual.description.length).toBeLessThanOrEqual(155)
    }
  })
})

describe('rewriteTarget with a media resolver', () => {
  it('uses the resolved URL for a repo asset, keeping no anchor when there is none', () => {
    const media = (repoPath: string) => `/assets/${repoPath.split('/').pop()}`
    expect(rewriteTarget('../assets/demos/x.gif', 'docs/cli.md', '/', media)).toEqual({ url: '/assets/x.gif', asset: 'assets/demos/x.gif' })
  })
})

describe('buildManual', () => {
  it('applies the stored theme to the light/dark sources at the end of the body', () => {
    const cli = MANUALS.find((m) => m.source === 'docs/cli.md')!
    const { html } = buildManual(cli, '# Command line guide\n', options)
    expect(html).toContain('source[media*="prefers-color-scheme"]')
    expect(html.indexOf('source[media*=')).toBeGreaterThan(html.indexOf('<main'))
  })

  it('builds an English page with one h1, its canonical and the social tags', () => {
    const cli = MANUALS.find((m) => m.source === 'docs/cli.md')!
    const { html } = buildManual(cli, '# Command line guide\n\nText.\n\n## Usage\n', options)
    expect(html).toContain('<html lang="en">')
    expect(html).toContain(`<title>${cli.title}</title>`)
    expect(html).toContain(`<link rel="canonical" href="https://mangadownloader.cat/docs/cli/" />`)
    expect(html).toContain(`<meta property="og:title" content="${cli.title}" />`)
    expect(html.match(/<h1[\s>]/g)).toHaveLength(1)
    expect(html).toContain('href="/"')
  })
})
