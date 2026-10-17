import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { ChapterBox, ChapterTitle, LOCKED_HINT, partsCount } from './ChapterPanel';

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

describe('partsCount', () => {
  it('says whether a chapter goes whole, in part, or not at all', () => {
    expect(partsCount(0, 2)).toBe('none selected');
    expect(partsCount(2, 2)).toBe('whole chapter');
    expect(partsCount(1, 2)).toBe('1 of 2 parts');
  });
});
