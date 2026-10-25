// Renders the per-language social cards (1280x640) from the master SVG and card.html.
// Usage from website/: node build/opengraph.mjs [--out DIR] [lang...] (default: es ca ja).
import { execFileSync } from 'node:child_process'
import { copyFileSync, existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const master = path.join(root, '..', 'assets', 'brand', 'social', 'opengraph.svg')
const template = path.join(root, 'build', 'opengraph', 'card.html')
const chipsFile = path.join(root, 'build', 'opengraph', 'chips.json')
const nodeModules = path.join(root, 'node_modules')

// Fonts per language: Fredoka for the Latin pages (as the master); Japanese uses the ja page's own fonts.
const FONTS = {
  latin: {
    css: [path.join(nodeModules, '@fontsource-variable/fredoka/wght.css')],
    tagline: { family: "'Fredoka Variable'", weight: 500 },
    chip: "'Fredoka Variable'",
  },
  ja: {
    css: [path.join(nodeModules, '@fontsource-variable/murecho/wght.css'), path.join(nodeModules, '@fontsource/m-plus-rounded-1c/700.css')],
    tagline: { family: "'M PLUS Rounded 1c'", weight: 700 },
    chip: "'Murecho Variable'",
  },
}

const args = process.argv.slice(2)
const outIdx = args.indexOf('--out')
const outDir = outIdx >= 0 ? path.resolve(args.splice(outIdx, 2)[1]) : path.join(root, 'public')
const langs = args.length ? args : ['es', 'ca', 'ja']

const escapeHtml = (s) => s.replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c])

// The master's artwork without the tagline and the chips: from the first tagline path up to the mini mark.
function artwork() {
  const svg = readFileSync(master, 'utf8')
  const start = svg.indexOf('<path fill="#4A4466"')
  const end = svg.indexOf('<g transform="translate(88 570) scale(0.1172)">')
  if (start < 0 || end < start) throw new Error('opengraph: the master SVG layout changed; update the markers')
  const art = svg.slice(0, start) + svg.slice(end)
  return art.replace(/<svg /, '<svg class="art" width="1280" height="640" ')
}

// The social title after "manga downloader: " (or the Japanese full-width colon), first letter capitalised.
function tagline(lang) {
  const locale = JSON.parse(readFileSync(path.join(root, 'locales', `${lang}.json`), 'utf8'))
  const title = locale.meta.socialTitle.split(/[:：]\s*/).slice(1).join(': ')
  return title.charAt(0).toUpperCase() + title.slice(1)
}

// The master's arrow stroke in its own box: the path's bbox grown by half the stroke, for the round caps.
const ARROW_BOX = '341.93 399.54 35.8 15.14'
function arrow() {
  const svg = readFileSync(master, 'utf8')
  const start = svg.indexOf('<path fill="none" stroke="#4A4466"')
  if (start < 0) throw new Error('opengraph: the master SVG has no tagline arrow')
  const path = svg.slice(start, svg.indexOf('/>', start) + 2)
  return `<svg class="arrow" viewBox="${ARROW_BOX}" width="35.8" height="15.14" aria-hidden="true">${path}</svg>`
}

// The tagline with the master's arrow in place of the "→" character.
function taglineHtml(lang) {
  const parts = tagline(lang).split('→')
  if (parts.length !== 2) throw new Error(`opengraph: ${lang} tagline needs exactly one arrow`)
  return escapeHtml(parts[0].trim()) + arrow() + escapeHtml(parts[1].trim())
}

function page(lang, art) {
  const fonts = lang === 'ja' ? FONTS.ja : FONTS.latin
  const chips = JSON.parse(readFileSync(chipsFile, 'utf8'))[lang]
  if (!chips || chips.length !== 4) throw new Error(`opengraph: ${lang} needs four chips`)
  const html = readFileSync(template, 'utf8')
  const stylesheets = fonts.css.map((file) => `<link rel="stylesheet" href="file://${file}" />`).join('\n    ')
  const replacements = {
    '%LANG%': lang,
    '%TITLE%': 'manga downloader',
    '%STYLESHEETS%': stylesheets,
    '%TAGLINE_FONT%': fonts.tagline.family,
    '%TAGLINE_WEIGHT%': String(fonts.tagline.weight),
    '%CHIP_FONT%': fonts.chip,
    '%ARTWORK%': art,
    '%TAGLINE%': taglineHtml(lang),
    '%CHIPS%': chips.map((chip) => `<span class="chip"><span class="dot"></span>${escapeHtml(chip)}</span>`).join(''),
  }
  return Object.entries(replacements).reduce((out, [key, value]) => out.split(key).join(value), html)
}

function screenshot(htmlFile, pngFile) {
  const chrome = process.env.CHROME ?? 'chromium'
  const env = { ...process.env }
  delete env.DISPLAY
  delete env.WAYLAND_DISPLAY
  // --virtual-time-budget lets the fonts load and the tagline fit before the capture.
  execFileSync(
    chrome,
    [
      '--headless=new',
      '--no-sandbox',
      '--disable-gpu',
      '--hide-scrollbars',
      '--allow-file-access-from-files',
      `--user-data-dir=${path.dirname(pngFile)}/profile`,
      '--window-size=1280,640',
      '--virtual-time-budget=10000',
      `--screenshot=${pngFile}`,
      `file://${htmlFile}`,
    ],
    { env, stdio: ['ignore', 'ignore', 'inherit'] },
  )
}

function shrink(pngFile, target) {
  try {
    execFileSync('magick', [pngFile, '-strip', '-colors', '256', '-dither', 'None', '-define', 'png:compression-level=9', target], { stdio: 'ignore' })
  } catch {
    copyFileSync(pngFile, target)
  }
}

const work = mkdtempSync(path.join(tmpdir(), 'og-cards-'))
try {
  const art = artwork()
  for (const lang of langs) {
    const htmlFile = path.join(work, `${lang}.html`)
    const rawPng = path.join(work, `${lang}.png`)
    writeFileSync(htmlFile, page(lang, art))
    screenshot(htmlFile, rawPng)
    const target = path.join(outDir, `opengraph-${lang}.png`)
    if (!existsSync(outDir)) throw new Error(`opengraph: no output folder ${outDir}`)
    shrink(rawPng, target)
    console.log(`wrote ${target}`)
  }
} finally {
  rmSync(work, { recursive: true, force: true })
}
