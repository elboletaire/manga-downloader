import { describe, expect, it } from 'vitest';
import { CATALOG, createT, createTemplate, fill, LANG_INFO, loadCatalog, resolveLanguage, say } from './index';

describe('resolveLanguage', () => {
  it('takes the saved language over the system', () => {
    expect(resolveLanguage('es', ['ca-ES'])).toBe('es');
    expect(resolveLanguage('ja', ['en-US'])).toBe('ja');
  });

  it('follows the system by primary subtag, first supported wins', () => {
    expect(resolveLanguage('', ['ca-ES-valencia'])).toBe('ca');
    expect(resolveLanguage('', ['fr-FR', 'es-MX', 'en-US'])).toBe('es');
    expect(resolveLanguage('', ['EN-gb'])).toBe('en');
    expect(resolveLanguage('', ['ja-JP', 'en-US'])).toBe('ja');
  });

  it('falls back to English when nothing is supported', () => {
    expect(resolveLanguage('', ['fr-FR'])).toBe('en');
    expect(resolveLanguage('', [])).toBe('en');
  });
});

describe('fill', () => {
  it('fills known placeholders and keeps unknown ones', () => {
    expect(fill('Download for {{os}}', { os: 'Linux' })).toBe('Download for Linux');
    expect(fill('{{a}} {{b}}', { a: 1 })).toBe('1 {{b}}');
    expect(fill('{{constructor}}', {})).toBe('{{constructor}}');
  });
});

describe('createT', () => {
  const catalog = {
    en: {
      'count.chapter_one': '{{count}} chapter',
      'count.chapter_other': '{{count}} chapters',
      'app.title': 'Manga Downloader',
      'app.only_en': 'English only',
    },
    es: {
      'count.chapter_one': '{{count}} capítulo',
      'count.chapter_other': '{{count}} capítulos',
      'app.title': 'Descargador',
    },
  };

  it('picks the plural form by count, falling back to _other', () => {
    expect(createT('en', catalog)('count.chapter', { count: 1 })).toBe('1 chapter');
    expect(createT('en', catalog)('count.chapter', { count: 0 })).toBe('0 chapters');
    expect(createT('es', catalog)('count.chapter', { count: 1000000 })).toBe('1000000 capítulos');
  });

  it('uses the language, then English, then the key', () => {
    const es = createT('es', catalog);
    expect(es('app.title')).toBe('Descargador');
    expect(es('app.only_en')).toBe('English only');
    expect(es('app.missing')).toBe('app.missing');
    expect(es('count.missing', { count: 2 })).toBe('count.missing');
  });

  it('fills the variables of the picked form', () => {
    expect(createT('es', catalog)('count.chapter', { count: 3 })).toBe('3 capítulos');
  });

  it('translates from the bundled catalog', () => {
    expect(createT('es')('common.close')).toBe('Cerrar');
    expect(createT('ca')('count.chapter', { count: 2 })).toBe('2 capítols');
    expect(createT('en')('count.chapter', { count: 1 })).toBe('1 chapter');
    expect(CATALOG.en?.['common.close']).toBe('Close');
  });

  it('translates into Japanese, whose one plural form is _other', () => {
    expect(createT('ja')('common.close')).toBe('閉じる');
    expect(createT('ja')('count.chapter', { count: 1 })).toBe('1話');
    expect(createT('ja')('count.chapter', { count: 12 })).toBe('12話');
    expect(LANG_INFO.ja.name).toBe('日本語');
  });

  it('builds the catalog from namespace files, nested keys included', () => {
    const built = loadCatalog({
      '../locales/en/app.json': { title: 'Title', hint: { short: 'Short' } },
      '../locales/xx/app.json': { title: 'ignored' },
    });
    expect(built).toEqual({ en: { 'app.title': 'Title', 'app.hint.short': 'Short' } });
  });
});

describe('createTemplate', () => {
  it('returns the message with its placeholders unfilled', () => {
    const catalog = { en: { 'app.hi': 'Hi {{name}}' } };
    expect(createTemplate('en', catalog)('app.hi', { name: 'x' })).toBe('Hi {{name}}');
  });
});

describe('say', () => {
  const es = createT('es');

  it('translates a message with its args', () => {
    const msg = { key: 'log.loaded', args: { title: 'Naruto', count: 2 }, text: 'Loaded Naruto: 2 chapters' };
    expect(say(es, msg, 'Loaded Naruto: 2 chapters')).toBe('Se ha cargado Naruto: 2 capítulos');
  });

  it('falls back to the message text for a key the catalog lacks', () => {
    const msg = { key: 'log.nope', text: 'English text' };
    expect(say(es, msg, 'plain')).toBe('English text');
  });

  it('uses the plain text when there is no message', () => {
    expect(say(es, undefined, 'plain')).toBe('plain');
  });
});
