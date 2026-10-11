// Build-time parser for the README's "Supported sites" section, so the
// website's list can never drift from it.

export interface Site {
  /** The domain as the README writes it, e.g. "asurascans.com". */
  domain: string
  /** The parenthesised note, e.g. "Asura Scans, former asuratoon.com". */
  note: string
  url: string
  /** Marked with `*` in the README: needs a Chromium-based browser. */
  browser: boolean
}

export interface BadgeLabels { label: string; srPrefix: string; title: string }
const EN_BADGE: BadgeLabels = { label: 'browser', srPrefix: 'needs a', title: 'Needs Chrome, Chromium, Brave or Edge' }

const ITEM = /^\s*[-*]\s+\[(.+)\]\((\S+?)\)\s*(\\?\*)?\s*$/

/** Parses every list item between "## Supported sites" and the next "## ". */
export function parseSites(readme: string): Site[] {
  const lines = readme.split(/\r?\n/)
  const start = lines.findIndex((l) => /^##\s+Supported sites\s*$/i.test(l))
  if (start === -1) throw new Error('README has no "## Supported sites" section')

  const sites: Site[] = []
  for (const line of lines.slice(start + 1)) {
    if (/^##\s/.test(line)) break
    const m = ITEM.exec(line)
    if (!m) continue
    const [, text, url, star] = m
    const paren = /^(\S+)\s*\((.*)\)$/.exec(text.trim())
    sites.push({
      domain: paren ? paren[1] : text.trim(),
      note: paren ? paren[2] : '',
      url,
      browser: Boolean(star),
    })
  }
  if (sites.length === 0) throw new Error('README\'s "Supported sites" section lists no sites')
  return sites
}

const escapeHTML = (s: string) =>
  s.replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]!)

/** Renders the sites as `<li>`s; `data-search` is what the filter matches. */
export function renderSites(sites: Site[], badge: BadgeLabels = EN_BADGE): string {
  return sites
    .map((s) => {
      const search = `${s.domain} ${s.note}`.toLowerCase()
      const badgeHtml = s.browser
        ? `<span class="site-badge" title="${escapeHTML(badge.title)}"><span class="visually-hidden">${escapeHTML(badge.srPrefix)} </span>${escapeHTML(badge.label)}</span>`
        : ''
      const note = s.note ? `<span class="site-note">${escapeHTML(s.note)}</span>` : ''
      return (
        `<li class="site" data-search="${escapeHTML(search)}"${s.browser ? ' data-browser' : ''}>` +
        `<a href="${escapeHTML(s.url)}" rel="nofollow noopener" target="_blank">` +
        `<span class="site-domain">${escapeHTML(s.domain)}</span>${badgeHtml}</a>${note}</li>`
      )
    })
    .join('\n')
}
