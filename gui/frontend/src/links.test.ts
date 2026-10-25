import type { MouseEvent } from 'react';
import { describe, expect, it } from 'vitest';
import { KOFI_URL, linkClick } from './links';

describe('linkClick', () => {
  it('opens the Ko-fi page outside the window', () => {
    const opened: string[] = [];
    const event = { preventDefault: () => undefined } as unknown as MouseEvent<HTMLAnchorElement>;
    linkClick(KOFI_URL, (url) => opened.push(url))(event);
    expect(opened).toEqual(['https://ko-fi.com/elboletaire']);
  });
});
