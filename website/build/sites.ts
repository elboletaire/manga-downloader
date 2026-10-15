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
  /** Last smoke-test outcome from the "Last verified" column. */
  verified: Verified
}

export type Verified =
  | { status: 'ok'; date: string }
  | { status: 'failing'; date: string }
  | { status: 'unknown' }

export interface BadgeLabels { label: string; srPrefix: string; title: string }
export const EN_BADGE: BadgeLabels = { label: 'browser', srPrefix: 'needs a', title: 'Needs Chrome, Chromium, Brave or Edge' }

// | [domain](url) \* | note | last verified |
const ROW = /^\|\s*\[(.+?)\]\((\S+?)\)\s*(\\?\*)?\s*\|(.*)\|(.*)\|\s*$/

/** Reads the "Last verified" cell: a date, "❌ failing since <date>" or anything else (unknown). */
export function parseVerified(cell: string): Verified {
  const c = cell.trim()
  const failing = /^❌\s*failing since (\d{4}-\d{2}-\d{2})$/.exec(c)
  if (failing) return { status: 'failing', date: failing[1] }
  if (/^\d{4}-\d{2}-\d{2}$/.test(c)) return { status: 'ok', date: c }
  return { status: 'unknown' }
}

/** Parses every table row between "## Supported sites" and the next "## ". */
export function parseSites(readme: string): Site[] {
  const lines = readme.split(/\r?\n/)
  const start = lines.findIndex((l) => /^##\s+Supported sites\s*$/i.test(l))
  if (start === -1) throw new Error('README has no "## Supported sites" section')

  const sites: Site[] = []
  for (const line of lines.slice(start + 1)) {
    if (/^##\s/.test(line)) break
    const m = ROW.exec(line)
    if (!m) continue
    const [, domain, url, star, note, verified] = m
    sites.push({ domain, note: note.trim(), url, browser: Boolean(star), verified: parseVerified(verified) })
  }
  if (sites.length === 0) throw new Error('README\'s "Supported sites" section lists no sites')
  return sites
}

const escapeHTML = (s: string) =>
  s.replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]!)

/** Labels for the verified line; `{{date}}` is replaced. */
export interface VerifiedLabels { on: string; failing: string; unknown: string }
export const EN_VERIFIED: VerifiedLabels = { on: 'Verified {{date}}', failing: 'Failing since {{date}}', unknown: 'Not checked' }

/** Formats a README date (YYYY-MM-DD) for a locale, at build time so the page needs no JS and never goes stale. */
export function formatDate(iso: string, locale = 'en'): string {
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeZone: 'UTC' }).format(new Date(`${iso}T00:00:00Z`))
}

/** Renders the sites as `<li>`s; `data-search` is what the filter matches. */
export function renderSites(sites: Site[], badge: BadgeLabels = EN_BADGE, verified: VerifiedLabels = EN_VERIFIED, locale = 'en'): string {
  return sites
    .map((s) => {
      const search = `${s.domain} ${s.note}`.toLowerCase()
      const badgeHtml = s.browser
        ? `<span class="site-badge" title="${escapeHTML(badge.title)}"><span class="visually-hidden">${escapeHTML(badge.srPrefix)} </span>${escapeHTML(badge.label)}</span>`
        : ''
      const note = s.note ? `<span class="site-note">${escapeHTML(s.note)}</span>` : ''
      const v = s.verified
      // The visible line is the accessible name too; the mark is decorative.
      const date = v.status === 'unknown' ? '' : `<time datetime="${v.date}">${escapeHTML(formatDate(v.date, locale))}</time>`
      const template = (v.status === 'ok' ? verified.on : v.status === 'failing' ? verified.failing : verified.unknown)
      if (v.status !== 'unknown' && !template.includes('{{date}}')) throw new Error(`verified label lacks {{date}}: ${template}`)
      const [before, after] = escapeHTML(template.replace('{{date}}', '\u0000')).split('\u0000')
      const mark = { ok: '\u2713', failing: '\u2715', unknown: '\u2013' }[v.status]
      const verifiedHtml =
        `<span class="site-verified" data-verified="${v.status}">` +
        `<span class="site-mark" aria-hidden="true">${mark}</span> ${before}${date}${after ?? ''}</span>`
      return (
        `<li class="site" data-search="${escapeHTML(search)}"${s.browser ? ' data-browser' : ''}>` +
        `<a href="${escapeHTML(s.url)}" rel="nofollow noopener" target="_blank">` +
        `<span class="site-domain">${escapeHTML(s.domain)}</span>${badgeHtml}</a>${note}${verifiedHtml}</li>`
      )
    })
    .join('\n')
}
