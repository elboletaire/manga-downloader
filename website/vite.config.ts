/// <reference types="vitest/config" />
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { defineConfig, type Plugin } from 'vite'
import { parseSites, renderSites } from './build/sites.ts'

// Where the site is served from. GitHub Pages serves a project site under
// /<repo>/; with a custom domain, set BASE_PATH=/ and SITE_URL to it.
const base = process.env.BASE_PATH ?? '/manga-downloader/'
const siteURL = process.env.SITE_URL ?? `https://elboletaire.github.io${base}`

const readmePath = fileURLToPath(new URL('../README.md', import.meta.url))

/** Fills index.html's placeholders, including the sites list from the README. */
function siteData(): Plugin {
  return {
    name: 'site-data',
    configureServer(server) {
      server.watcher.add(readmePath)
      server.watcher.on('change', (file) => {
        if (file === readmePath) server.ws.send({ type: 'full-reload' })
      })
    },
    generateBundle() {
      // Crawl files, generated so they follow SITE_URL.
      this.emitFile({ type: 'asset', fileName: 'robots.txt', source: `User-agent: *\nAllow: /\n\nSitemap: ${siteURL}sitemap.xml\n` })
      this.emitFile({
        type: 'asset',
        fileName: 'sitemap.xml',
        source:
          '<?xml version="1.0" encoding="UTF-8"?>\n' +
          '<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n' +
          `  <url><loc>${siteURL}</loc></url>\n` +
          '</urlset>\n',
      })
    },
    transformIndexHtml(html, ctx) {
      // Preload the two Latin font files the first screen needs, so text
      // doesn't reflow (and shift the page) when they swap in.
      const fonts = Object.keys(ctx.bundle ?? {}).filter((f) => /(inter|fredoka)-latin-wght-normal-.*\.woff2$/.test(f))
      const preloads = fonts
        .map((f) => `<link rel="preload" href="${base}${f}" as="font" type="font/woff2" crossorigin />`)
        .join('\n    ')
      const sites = parseSites(readFileSync(readmePath, 'utf8'))
      return html
        .replace('<!-- sites:list -->', renderSites(sites))
        .replaceAll('%SITES_COUNT%', String(sites.length))
        .replaceAll('%OTHER_SITES_COUNT%', String(sites.length - 1))
        .replaceAll('%BROWSER_COUNT%', String(sites.filter((s) => s.browser).length))
        .replaceAll('%SITE_URL%', siteURL)
        .replace('<!-- font-preloads -->', preloads)
    },
  }
}

export default defineConfig({
  base,
  plugins: [siteData()],
  build: {
    // Never inline assets as data: URIs. Firefox doesn't render the SVG data:
    // URI in the dark <picture> source, so the hero mascot disappeared there.
    assetsInlineLimit: 0,
  },
  test: {
    include: ['src/**/*.test.ts', 'build/**/*.test.ts'],
  },
})
