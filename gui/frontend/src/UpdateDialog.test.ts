import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { UpdateDialog } from './UpdateDialog';

describe('UpdateDialog', () => {
  it('names both versions and offers the three choices', () => {
    const html = renderToStaticMarkup(
      createElement(UpdateDialog, {
        latest: 'v1.13.0',
        current: 'v1.12.0',
        onUpdate: () => Promise.resolve(),
        onReleaseNotes: () => undefined,
        onClose: () => undefined,
      }),
    );
    expect(html).toContain('Update available');
    expect(html).toContain('is available (you have');
    expect(html).toContain('Update now');
    expect(html).toContain('Release notes');
    expect(html).toContain('Later');
  });
});
