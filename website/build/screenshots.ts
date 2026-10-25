// Build-time helper: the translated pages show their own screenshots when the
// translated WebP exists, and the English one per image when it doesn't.

const DIR = '/src/assets/screenshots/'

/**
 * Points each `/src/assets/screenshots/<name>.webp` in `html` at `<lang>/<name>.webp`
 * when `exists` says that URL is there. The English page is returned as it is.
 */
export function localizeScreenshots(html: string, lang: string, exists: (url: string) => boolean): string {
  if (lang === 'en') return html
  return html.replace(/\/src\/assets\/screenshots\/([\w-]+\.webp)/g, (url, name: string) => {
    const localized = `${DIR}${lang}/${name}`
    return exists(localized) ? localized : url
  })
}
