import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { ChapterTitle, LOCKED_HINT } from './ChapterPanel';

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
