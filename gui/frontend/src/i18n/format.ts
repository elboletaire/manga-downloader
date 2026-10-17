// Numbers in the language's own style. Chapter numbers stay as they are.

/** A count with the language's digit grouping, e.g. 1,234 in English, 1.234 in Spanish. */
export function count(n: number, lang: string): string {
  return new Intl.NumberFormat(lang).format(n);
}

/** A 0-1 fraction as a whole percentage. */
export function percent(fraction: number, lang: string): string {
  return new Intl.NumberFormat(lang, { style: 'percent' }).format(fraction);
}
