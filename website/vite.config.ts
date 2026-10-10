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
          `  <url><loc>${siteURL}</loc><lastmod>${new Date().toISOString().slice(0, 10)}</lastmod></url>\n` +
          '</urlset>\n',
      })
    },
    transformIndexHtml(html) {
      const sites = parseSites(readFileSync(readmePath, 'utf8'))
      return html
        .replace('<!-- sites:list -->', renderSites(sites))
        .replaceAll('%SITES_COUNT%', String(sites.length))
        .replaceAll('%OTHER_SITES_COUNT%', String(sites.length - 1))
        .replaceAll('%BROWSER_COUNT%', String(sites.filter((s) => s.browser).length))
        .replaceAll('%SITE_URL%', siteURL)
    },
  }
}

export default defineConfig({
  base,
  plugins: [siteData()],
  build: {
    // The mark SVGs are tiny; inlining them saves requests above the fold.
    assetsInlineLimit: 4096,
  },
  test: {
    include: ['src/**/*.test.ts', 'build/**/*.test.ts'],
  },
})
