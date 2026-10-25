/// <reference types="vitest/config" />
import { existsSync, readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import i18next from 'i18next'
import { defineConfig, type Plugin } from 'vite'
import { localizeScreenshots } from './build/screenshots.ts'
import { parseSites, renderSites } from './build/sites.ts'
import { themeScript } from './src/theme.ts'
import { LANG_INFO, LANGS, type Lang } from './src/lang.ts'
import { fetchStats, renderStats, type Stats } from './build/stats.ts'

// Where the site is served from: the custom domain at its root. Without one,
// GitHub Pages serves a project site under /<repo>/ (BASE_PATH=/manga-downloader/).
const base = process.env.BASE_PATH ?? '/'
const siteURL = process.env.SITE_URL ?? `https://mangadownloader.cat${base}`

const root = fileURLToPath(new URL('.', import.meta.url))
const readmePath = fileURLToPath(new URL('../README.md', import.meta.url))
const templatePath = path.join(root, 'index.html')
const localesDir = path.join(root, 'locales')

// Values every translation can interpolate besides the counts.
const GLOBALS = {
  repo: 'https://github.com/elboletaire/manga-downloader',
  author: 'https://github.com/elboletaire',
  filenameTemplate: '{{.Series}} {{.Number}}',
}

type Tree = { [key: string]: string | Tree }

// Fetched once per build (or dev server), shared by every language's page.
let statsPromise: Promise<Stats | null> | undefined

const pageURL = (lang: Lang) => siteURL + LANG_INFO[lang].path
const readLocale = (lang: Lang) => JSON.parse(readFileSync(path.join(localesDir, `${lang}.json`), 'utf8')) as Tree
const escapeAttr = (s: string) =>
  s.replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c]!)

/** index.html is the English page; <lang>/index.html are stubs rendered from it. */
function pageLang(filename: string): Lang {
  const rel = path.relative(root, filename).split(path.sep).join('/')
  const lang = LANGS.find((l) => `${LANG_INFO[l].path}index.html` === rel)
  if (!lang) throw new Error(`site-data: no language for ${rel}`)
  return lang
}

/** Hidden banners, one per other language: main.ts shows the one matching the visitor's preferred language. */
function banners(lang: Lang, t: (key: string) => string): string {
  return LANGS.filter((l) => l !== lang)
    .map((l) => {
      const text = t(`languageBanner.${l}.text`)
      return (
        `<aside class="lang-banner" lang="${l}" data-lang-banner="${l}" aria-label="${escapeAttr(text)}" hidden>` +
        `<p>${text} <a href="${base}${LANG_INFO[l].path}">${t(`languageBanner.${l}.link`)}</a></p>` +
        `<button type="button" class="lang-banner-close" data-lang-banner-close aria-label="${escapeAttr(t(`languageBanner.${l}.close`))}">` +
        '<svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>' +
        '</button></aside>'
      )
    })
    .join('')
}

function alternates(): string {
  const links = LANGS.map((l) => `<link rel="alternate" hreflang="${l}" href="${pageURL(l)}" />`)
  links.push(`<link rel="alternate" hreflang="x-default" href="${pageURL('en')}" />`)
  return links.join('\n    ')
}

function ogAlternates(lang: Lang): string {
  return LANGS.filter((l) => l !== lang)
    .map((l) => `<meta property="og:locale:alternate" content="${LANG_INFO[l].ogLocale}" />`)
    .join('\n    ')
}

function switcher(lang: Lang): string {
  return LANGS.map((l) => {
    const current = l === lang ? ' aria-current="page"' : ''
    return (
      `<a href="${base}${LANG_INFO[l].path}" hreflang="${l}" lang="${l}" data-lang="${l}"${current}>` +
      `<span aria-hidden="true">${l.toUpperCase()}</span><span class="visually-hidden">${LANG_INFO[l].name}</span></a>`
    )
  }).join('')
}

function sitemap(): string {
  const lastmod = new Date().toISOString().slice(0, 10)
  const alts = [...LANGS.map((l) => [l, pageURL(l)]), ['x-default', pageURL('en')]]
    .map(([l, href]) => `    <xhtml:link rel="alternate" hreflang="${l}" href="${href}" />`)
    .join('\n')
  const urls = LANGS.map((l) => `  <url>\n    <loc>${pageURL(l)}</loc>\n    <lastmod>${lastmod}</lastmod>\n${alts}\n  </url>`).join('\n')
  return (
    '<?xml version="1.0" encoding="UTF-8"?>\n' +
    '<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">\n' +
    `${urls}\n</urlset>\n`
  )
}

/** Renders the template for one language: translations, counts, sites, SEO tags. */
async function renderPage(html: string, lang: Lang): Promise<string> {
  const sites = parseSites(readFileSync(readmePath, 'utf8'))
  const counts: Record<string, number> = {
    sites: sites.length,
    others: sites.length - 1,
    browsers: sites.filter((s) => s.browser).length,
  }
  const resources = Object.fromEntries(LANGS.map((l) => [l, { translation: readLocale(l) }]))
  const i18n = i18next.createInstance()
  await i18n.init({ lng: lang, fallbackLng: false, resources, interpolation: { escapeValue: false } })
  const translate = i18n.t.bind(i18n) as unknown as (key: string, options?: object) => string

  // `keepDate` leaves "{{date}}" for renderSites to fill in per site.
  const t = (key: string, countName?: string, keepDate = false): string => {
    const options: Record<string, string | number> = { ...GLOBALS, ...counts }
    if (keepDate) options.date = '{{date}}'
    if (countName) {
      if (!(countName in counts)) throw new Error(`site-data: unknown count "${countName}" in ${key}`)
      options.count = counts[countName]
    }
    if (!i18n.exists(key, options)) throw new Error(`site-data: missing translation "${key}" for ${lang}`)
    return String(translate(key, options))
  }

  const badge = { label: t('sites.badge.label'), srPrefix: t('sites.badge.srPrefix'), title: t('sites.badge.title') }
  const verified = { on: t('sites.verified.on', undefined, true), failing: t('sites.verified.failing', undefined, true), unknown: t('sites.verified.unknown') }
  const runtime = JSON.stringify(readLocale(lang).runtime).replace(/</g, '\\u003c')
  statsPromise ??= fetchStats()
  const statsObj = await statsPromise
  const stats = renderStats(statsObj, { downloads: t('stats.downloads'), dockerPulls: t('stats.dockerPulls') })

  // Embed latest release data for the download button, with < escaped to prevent HTML injection
  const releaseData = statsObj?.latest
    ? JSON.stringify(statsObj.latest).replace(/</g, '\\u003c')
    : ''

  // Function replacers, so a "$" in the inserted text is never a replacement pattern.
  // Translated screenshots where their WebP exists, else the English one.
  return localizeScreenshots(html, lang, (url) => existsSync(path.join(root, url)))
    .replace('<!-- theme:init -->', () => themeScript())
    .replace('<!-- fonts:ja -->', () => (lang === 'ja' ? '<link rel="stylesheet" href="/src/japanese-fonts.css" />' : ''))
    .replace('<!-- i18n:alternates -->', () => alternates())
    .replace('<!-- i18n:og-alternates -->', () => ogAlternates(lang))
    .replace('<!-- i18n:switcher -->', () => switcher(lang))
    .replace('<!-- i18n:banners -->', () => banners(lang, (key) => t(key)))
    .replace('<!-- i18n:runtime -->', () => `<script type="application/json" id="i18n-runtime">${runtime}</script>`)
    .replace('<!-- release:data -->', () => (releaseData ? `<script type="application/json" id="latest-release">${releaseData}</script>` : ''))
    .replace('<!-- sites:list -->', () => renderSites(sites, badge, verified, lang))
    .replace(/<!-- (stats:\w+) -->/g, (_m, key: string) => stats[key] ?? '')
    .replace(/\{\{(t|a|j):([\w.]+)(?::(\w+))?\}\}/g, (_m, kind: string, key: string, countName?: string) => {
      const text = t(key, countName)
      if (kind === 'a') return escapeAttr(text)
      if (kind === 'j') return JSON.stringify(text).slice(1, -1)
      return text
    })
    .replaceAll('%PAGE_LANG%', lang)
    .replaceAll('%PAGE_URL%', pageURL(lang))
    .replaceAll('%OG_LOCALE%', LANG_INFO[lang].ogLocale)
    .replaceAll('%SITE_URL%', siteURL)
}

/** Served by GitHub Pages for any missing path: kept out of search, with the site's styles and theme, no page script. */
function notFoundPage(stylesheet: string): string {
  return `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <meta name="robots" content="noindex" />
    <title>Page not found · manga downloader</title>
    <meta name="color-scheme" content="light dark" />
    <link rel="icon" href="${base}favicon.svg" type="image/svg+xml" />
    <link rel="stylesheet" href="${base}${stylesheet}" />
    ${themeScript()}
  </head>
  <body>
    <header class="site-header">
      <div class="container header-inner">
        <a class="brand" href="${base}">manga downloader</a>
      </div>
    </header>
    <main class="container not-found">
      <p class="eyebrow">404</p>
      <h1>Page not found</h1>
      <p class="lede">This page doesn't exist or has moved. Pick a language to start again.</p>
      <ul class="not-found-links">
        <li><a href="${base}">English</a></li>
        <li><a href="${base}es/" lang="es">Español</a></li>
        <li><a href="${base}ca/" lang="ca">Català</a></li>
        <li><a href="${base}ja/" lang="ja">日本語</a></li>
      </ul>
    </main>
  </body>
</html>
`
}

/** Renders every language's page from index.html, plus robots.txt, sitemap.xml and 404.html. */
function siteData(): Plugin[] {
  return [
    {
      name: 'site-data',
      configureServer(server) {
        server.watcher.add([readmePath, localesDir])
        server.watcher.on('change', (file) => {
          if (file === readmePath || file.startsWith(localesDir)) server.ws.send({ type: 'full-reload' })
        })
      },
      generateBundle(_options, bundle) {
        // Crawl files, generated so they follow SITE_URL.
        this.emitFile({ type: 'asset', fileName: 'robots.txt', source: `User-agent: *\nAllow: /\n\nSitemap: ${siteURL}sitemap.xml\n` })
        this.emitFile({ type: 'asset', fileName: 'sitemap.xml', source: sitemap() })
        // The stylesheet's hashed name is only known from the bundle.
        const stylesheet = Object.keys(bundle).find((f) => f.endsWith('.css'))
        if (!stylesheet) throw new Error('site-data: no stylesheet for 404.html')
        this.emitFile({ type: 'asset', fileName: '404.html', source: notFoundPage(stylesheet) })
      },
      // "pre": runs before Vite parses the page for scripts and assets, in dev and build.
      transformIndexHtml: {
        order: 'pre',
        async handler(html, ctx) {
          const lang = pageLang(ctx.filename)
          return renderPage(lang === 'en' ? html : readFileSync(templatePath, 'utf8'), lang)
        },
      },
    },
    {
      // Needs the bundle, which only the normal-order hook gets (build only).
      name: 'site-data:fonts',
      transformIndexHtml(html, ctx) {
        // Preload the two Latin font files the first screen needs, so text
        // doesn't reflow (and shift the page) when they swap in.
        const fonts = Object.keys(ctx.bundle ?? {}).filter((f) => /(inter|fredoka)-latin-wght-normal-.*\.woff2$/.test(f))
        const preloads = fonts
          .map((f) => `<link rel="preload" href="${base}${f}" as="font" type="font/woff2" crossorigin />`)
          .join('\n    ')
        // The page's own hero WebP (translated when it exists), by its hashed name in the bundle; dev keeps the placeholder.
        const lang = html.match(/<html lang="([^"]+)"/)?.[1] ?? 'en'
        const hero = (source: string) =>
          Object.entries(ctx.bundle ?? {}).find(([, f]) => 'originalFileNames' in f && f.originalFileNames.includes(source))?.[0]
        const screenshot =
          hero(`src/assets/screenshots/${lang}/app-overview-light-1600.webp`) ??
          hero('src/assets/screenshots/app-overview-light-1600.webp')
        return html
          .replace('<!-- font-preloads -->', () => preloads)
          .replace('%SCREENSHOT%', () => (screenshot ? siteURL + screenshot : '%SCREENSHOT%'))
      },
    },
    {
      // Vite injects the stylesheet at the end of <head>, after ~10 KB of data
      // scripts, so slow connections paint unstyled first. Move it up to the
      // viewport meta; post order runs once Vite has injected it.
      name: 'site-data:early-css',
      transformIndexHtml: {
        order: 'post',
        handler(html) {
          const link = html.match(/<link rel="stylesheet"[^>]*>/)?.[0]
          if (!link) return html
          return html
            .replace(link, '')
            .replace(/(<meta name="viewport"[^>]*\/>)/, `$1\n    ${link}`)
        },
      },
    },
  ]
}

export default defineConfig({
  base,
  plugins: [siteData()],
  build: {
    // Never inline assets as data: URIs. Firefox doesn't render the SVG data:
    // URI in the dark <picture> source, so the hero mascot disappeared there.
    assetsInlineLimit: 0,
    // One page per language; es/, ca/ and ja/ index.html are stubs rendered from index.html.
    rolldownOptions: {
      input: Object.fromEntries(LANGS.map((l) => [l, path.join(root, LANG_INFO[l].path, 'index.html')])),
    },
  },
  test: {
    include: ['src/**/*.test.ts', 'build/**/*.test.ts'],
  },
})
