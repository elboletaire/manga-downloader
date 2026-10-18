import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { CATALOG, createT } from './i18n';
import { I18nProvider } from './i18n/react';
import { ChapterBox, ChapterPanel, ChapterTitle, RangeForm, canApplyRange, partsCount } from './ChapterPanel';
import { createMockApi } from './mock';

const LOCKED_HINT = createT('en')('chapters.locked');

function title(locked: boolean): string {
  return renderToStaticMarkup(createElement(ChapterTitle, { title: 'Chapter 186', locked }));
}

describe('ChapterTitle', () => {
  it('marks a locked chapter with a lock that names why, in the title', () => {
    const html = title(true);
    expect(html).toContain(`class="lock" role="img" title="${LOCKED_HINT}" aria-label="${LOCKED_HINT}"`);
    expect(html).toContain('class="title"');
    expect(html.indexOf('class="lock"')).toBeLessThan(html.indexOf('Chapter 186'));
  });

  it('shows a free chapter with no lock at all', () => {
    const html = title(false);
    expect(html).not.toContain('lock');
    expect(html).toBe('<span class="title">Chapter 186</span>');
  });
});

describe('ChapterBox', () => {
  const box = (on: boolean, locked: boolean) => renderToStaticMarkup(createElement(ChapterBox, { on, locked }));

  it('greys out the box of a locked chapter', () => {
    expect(box(false, true)).toBe('<span class="box disabled" aria-hidden="true"></span>');
  });

  it('leaves a free chapter\'s box as it was', () => {
    expect(box(false, false)).toBe('<span class="box" aria-hidden="true"></span>');
    expect(box(true, false)).toBe('<span class="box on" aria-hidden="true"></span>');
  });
});


describe('RangeForm', () => {
  const form = (text: string) =>
    renderToStaticMarkup(createElement(RangeForm, { text, bad: false, onText: () => {}, onApply: () => {} }));
  const plus = '<div class="input-action"><button type="submit" class="btn ghost small icon-only" aria-label="Add range to selection" title="Add range to selection"';

  it('shows the + inside the field only once there is text', () => {
    expect(form('')).not.toContain('input-action');
    expect(form('   ')).not.toContain('input-action');
    expect(form('1-10')).toContain(plus);
    expect(form('1-10')).toMatch(/<div class="input-wrap">.*<\/button><\/div><\/div>/);
  });

  it('applies Enter only when the box has a range in it', () => {
    expect(canApplyRange('   ')).toBe(false);
    expect(canApplyRange('')).toBe(false);
    expect(canApplyRange('1-10')).toBe(true);
  });
});

describe('partsCount', () => {
  it('says whether a chapter goes whole, in part, or not at all', () => {
    const t = createT('en');
    expect(partsCount(0, 2, t)).toBe('none selected');
    expect(partsCount(2, 2, t)).toBe('whole chapter');
    expect(partsCount(1, 2, t)).toBe('1 of 2 parts');
  });

  it('translates the header of a chapter with its parts picked', () => {
    expect(partsCount(1, 2, createT('es'))).toBe('1 de 2 partes');
    expect(partsCount(2, 2, createT('ca'))).toBe('capítol sencer');
  });
});

describe('ChapterTitle in another language', () => {
  it('names the lock in the language of the app', () => {
    const html = renderToStaticMarkup(
      createElement(
        I18nProvider,
        { pref: 'es', persist: async () => {}, catalog: CATALOG, systemLanguages: [] },
        createElement(ChapterTitle, { title: 'Capítulo 186', locked: true }),
      ),
    );
    expect(html).toContain('title="Bloqueado, así que no se puede seleccionar"');
  });
});

describe('ChapterPanel of a parts run', () => {
  it('heads a merged chapter with its title and count, and shows each part by its part alone', async () => {
    const api = createMockApi({ scenario: 'parts', uiLanguage: 'ca' });
    const state = await api.state();
    const chapters = await api.chapters();
    const noop = async () => {};
    const markup = renderToStaticMarkup(
      createElement(ChapterPanel, {
        state,
        chapters,
        selected: [],
        disabled: false,
        onToggle: noop,
        onToggleMany: noop,
        onSetAll: noop,
        onRange: async () => 0,
        onVolumes: noop,
        onReload: () => {},
        onError: () => {},
      }),
    );
    const first = chapters.find((c) => c.number === 2 && c.part === 'Part 1');
    expect(first).toBeDefined();
    const title = first?.wholeTitle ?? '';
    expect(title).not.toBe('');
    expect(markup).toContain(`Chapter 2 · ${escapeHtml(title)} · 2 parts`);
    expect(markup).toMatch(/class="[^"]*\bpart-row\b/);
    expect(markup).toMatch(/>Part 1</);
    expect(markup).toMatch(/>Part 2</);
    // a part row never repeats the chapter's title
    expect(markup).not.toContain(escapeHtml(first?.title ?? ''));
  });
});

function escapeHtml(text: string): string {
  return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#x27;');
}
