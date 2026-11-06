// The user manuals (docs/cli.md, docs/app.md) rendered to English pages at
// /docs/cli/ and /docs/app/: Markdown to HTML, links and images rewritten to
// where the site serves them, wrapped in the site's header and footer.

import path from 'node:path'
import { Marked, type Token } from 'marked'
import { forcedTheme, schemeMedia, THEME_KEY, themeScript } from '../src/theme.ts'

export const REPO = 'https://github.com/elboletaire/manga-downloader'

export interface Manual {
  /** The Markdown file, relative to the repo root. */
  source: string
  /** The page's path under the site root, with a trailing slash. */
  path: string
  title: string
  description: string
}

export const MANUALS: Manual[] = [
  {
    source: 'docs/cli.md',
    path: 'docs/cli/',
    title: 'manga downloader CLI manual: every flag and option',
    description: 'Every flag, option and environment variable of the manga-downloader command line, with examples for each feature.',
  },
  {
    source: 'docs/app.md',
    path: 'docs/app/',
    title: 'manga downloader desktop app manual: step by step',
    description: "How to load a series, pick chapters, pack them and set the app's options, from the first launch to the update check.",
  },
]

/** Where the manuals' images and links to repo assets are copied to: docs/media/<the path under assets/>. */
export const MEDIA = 'docs/media/'

export const mediaFile = (repoPath: string) => MEDIA + repoPath.slice('assets/'.length)

const EXTERNAL = /^([a-z][a-z\d+.-]*:|\/\/)/i

/** GitHub's heading anchor: lowercase, punctuation dropped, spaces to hyphens. */
export function slugify(text: string): string {
  return text.toLowerCase().replace(/[^\p{L}\p{N}\p{M} _-]/gu, '').trim().replace(/ /g, '-')
}

/** Anchors for one page: a repeated heading gets "-1", "-2" like on GitHub. */
function headingIds(): (text: string) => string {
  const seen = new Map<string, number>()
  return (text) => {
    const slug = slugify(text)
    const n = seen.get(slug) ?? 0
    seen.set(slug, n + 1)
    return n ? `${slug}-${n}` : slug
  }
}

/** Where one link or image target of a manual goes. `asset` is set when it needs copying. */
export function rewriteTarget(
  target: string,
  source: string,
  base: string,
  media: (repoPath: string) => string = (repoPath) => base + mediaFile(repoPath),
): { url: string; asset?: string } {
  if (target.startsWith('#') || EXTERNAL.test(target)) return { url: target }
  const hashAt = target.indexOf('#')
  const file = hashAt < 0 ? target : target.slice(0, hashAt)
  const hash = hashAt < 0 ? '' : target.slice(hashAt)
  const repoPath = path.posix.normalize(path.posix.join(path.posix.dirname(source), file))
  const manual = MANUALS.find((m) => m.source === repoPath)
  if (manual) return { url: `${base}${manual.path}${hash}` }
  if (repoPath.startsWith('assets/')) return { url: `${media(repoPath)}${hash}`, asset: repoPath }
  return { url: `${REPO}/blob/master/${repoPath}${hash}` }
}

/** Rewrites the href, src and srcset attributes of a rendered manual (raw HTML included). */
export function rewriteLinks(
  html: string,
  source: string,
  base: string,
  media?: (repoPath: string) => string,
): { html: string; assets: string[] } {
  const assets = new Set<string>()
  const rewrite = (url: string) => {
    const target = rewriteTarget(url, source, base, media)
    if (target.asset) assets.add(target.asset)
    return target.url
  }
  const out = html.replace(/\b(href|src|srcset)="([^"]*)"/g, (_match, attr: string, value: string) => {
    if (attr !== 'srcset') return `${attr}="${rewrite(value)}"`
    // Each candidate is "url [descriptor]"; only the url is rewritten.
    const candidates = value.split(',').map((c) => c.trim().replace(/^\S+/, rewrite))
    return `srcset="${candidates.join(', ')}"`
  })
  return { html: out, assets: [...assets] }
}

/** The text of inline tokens without their Markdown, which is what GitHub slugs. */
function plainText(tokens: Token[]): string {
  return tokens.map((t) => ('tokens' in t && t.tokens ? plainText(t.tokens) : ((t as { text?: string }).text ?? ''))).join('')
}

/** Markdown to HTML, with GFM tables and anchored headings; links are not rewritten yet. */
export function renderMarkdown(markdown: string): string {
  const id = headingIds()
  const marked = new Marked({
    renderer: {
      heading({ tokens, depth }) {
        return `<h${depth} id="${id(plainText(tokens))}">${this.parser.parseInline(tokens)}</h${depth}>\n`
      },
    },
  })
  return marked.parse(markdown, { async: false })
}

const brandPicture = (light: string, dark: string, width: number, height: number, alt: string, lazy: boolean) =>
  `<picture>\n<source srcset="${dark}" media="(prefers-color-scheme: dark)" />\n` +
  `<img src="${light}" alt="${alt}" width="${width}" height="${height}"${lazy ? ' loading="lazy"' : ''} />\n</picture>`

export interface PageOptions {
  /** The site's base path, with a trailing slash. */
  base: string
  /** The site's absolute URL, with a trailing slash. */
  siteURL: string
  /** The stylesheet's URL. */
  stylesheet: string
  /** The brand lockup, light and dark, as URLs. */
  brand: { light: string; dark: string }
  /** Where a repo asset is served from; defaults to its copy under docs/media/. */
  media?: (repoPath: string) => string
}

/**
 * Inline script at the end of <body>: applies the stored theme to the light/dark
 * sources and theme-color metas, which main.ts only does on the home page.
 */
function schemeScript(): string {
  return (
    '<script>(function(){try{' +
    `var t=(${forcedTheme.toString()})(localStorage.getItem(${JSON.stringify(THEME_KEY)}));` +
    'if(!t)return;' +
    `var nodes=document.querySelectorAll('source[media*="prefers-color-scheme"], meta[name="theme-color"][media*="prefers-color-scheme"]');` +
    'for(var i=0;i<nodes.length;i++){var m=nodes[i].getAttribute("media");' +
    `nodes[i].setAttribute("media",(${schemeMedia.toString()})(m,m.indexOf("dark")>=0?"dark":"light",t))}` +
    'var imgs=document.querySelectorAll("picture > img");' +
    'for(var j=0;j<imgs.length;j++)imgs[j].parentNode.insertBefore(imgs[j],imgs[j].nextSibling)' +
    '}catch(e){}})()</script>'
  )
}

/** Renders one manual to its full page. `assets` are the repo files its pages reference. */
export function buildManual(manual: Manual, markdown: string, options: PageOptions): { html: string; assets: string[] } {
  const { base, siteURL, stylesheet, brand, media } = options
  const { html: body, assets } = rewriteLinks(renderMarkdown(markdown), manual.source, base, media)
  const url = siteURL + manual.path
  const image = siteURL + 'opengraph.png'
  const html = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta http-equiv="content-language" content="en" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>${manual.title}</title>
    <meta name="description" content="${manual.description}" />
    <link rel="canonical" href="${url}" />
    ${themeScript()}
    <meta name="color-scheme" content="light dark" />
    <meta name="theme-color" content="#F6F2FF" media="(prefers-color-scheme: light)" />
    <meta name="theme-color" content="#16132A" media="(prefers-color-scheme: dark)" />

    <link rel="icon" href="${base}favicon.svg" type="image/svg+xml" />
    <link rel="stylesheet" href="${stylesheet}" />

    <meta property="og:type" content="website" />
    <meta property="og:site_name" content="manga downloader" />
    <meta property="og:locale" content="en_US" />
    <meta property="og:title" content="${manual.title}" />
    <meta property="og:description" content="${manual.description}" />
    <meta property="og:url" content="${url}" />
    <meta property="og:image" content="${image}" />
    <meta property="og:image:width" content="1280" />
    <meta property="og:image:height" content="640" />
    <meta property="og:image:alt" content="manga downloader" />
    <meta name="twitter:card" content="summary_large_image" />
    <meta name="twitter:title" content="${manual.title}" />
    <meta name="twitter:description" content="${manual.description}" />
    <meta name="twitter:image" content="${image}" />
  </head>
  <body>
    <a class="skip-link" href="#main">Skip to content</a>

    <header class="site-header">
      <div class="container header-inner">
        <a class="brand" href="${base}" aria-label="manga downloader home">
          ${brandPicture(brand.light, brand.dark, 199, 32, '', false)}
        </a>
        <nav class="site-nav doc-nav" aria-label="Manuals">
          <ul>
            <li><a href="${base}docs/cli/">CLI manual</a></li>
            <li><a href="${base}docs/app/">App manual</a></li>
          </ul>
        </nav>
      </div>
    </header>

    <main id="main" class="container">
      <article class="doc">
        ${body}
      </article>
    </main>

    <footer class="site-footer">
      <div class="container footer-inner">
        <div class="footer-brand">
          ${brandPicture(brand.light, brand.dark, 174, 28, 'manga downloader', true)}
          <p>Free and open source, under the <a href="${REPO}/blob/master/LICENSE">AGPL-3.0 licence</a>.</p>
          <p>Made by <a href="https://github.com/elboletaire">elboletaire</a>.</p>
        </div>
        <nav class="footer-nav" aria-label="Project">
          <ul>
            <li><a href="${base}">Home</a></li>
            <li><a href="${REPO}">GitHub</a></li>
            <li><a href="${REPO}/releases">Releases</a></li>
            <li><a href="${REPO}/issues">Report an issue</a></li>
          </ul>
        </nav>
      </div>
    </footer>
    ${schemeScript()}
  </body>
</html>
`
  return { html, assets }
}
