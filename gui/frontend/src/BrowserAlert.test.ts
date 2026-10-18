import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { I18nProvider } from './i18n/react';
import { BrowserAlert } from './BrowserAlert';

const props = { message: 'Solve the check in the browser window', onShow: () => undefined, onDismiss: () => undefined };

describe('BrowserAlert', () => {
  it('shows its title and buttons in English by default', () => {
    const html = renderToStaticMarkup(createElement(BrowserAlert, props));
    expect(html).toContain('The browser needs you');
    expect(html).toContain('Dismiss');
    expect(html).toContain('Show the browser window');
    expect(html).toContain('Solve the check in the browser window');
  });

  it('translates its title and buttons, and keeps the message from Go as it is', () => {
    const html = renderToStaticMarkup(
      createElement(
        I18nProvider,
        { pref: 'es', persist: async () => undefined, systemLanguages: ['en'] },
        createElement(BrowserAlert, props),
      ),
    );
    expect(html).toContain('El navegador te necesita');
    expect(html).toContain('Descartar');
    expect(html).toContain('Mostrar la ventana del navegador');
    expect(html).toContain('Solve the check in the browser window');
  });
});
