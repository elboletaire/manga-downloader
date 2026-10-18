import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { createWailsApi, type Api } from './api';
import { Root } from './Root';
import '@fontsource-variable/inter/wght.css';
import './styles.css';

// Inside the Go app window.go is set and the real backend answers. In a plain
// browser (pnpm dev) or with ?mock, the mock does. The mock (and its One Piece
// dataset) is only bundled in dev builds or when VITE_MOCK=1 is set at build
// time, so it is absent from normal production builds. Set VITE_MOCK=1 when
// building for the screenshot capture (e.g. VITE_MOCK=1 wails build ...).
const canMock = import.meta.env.DEV || import.meta.env.VITE_MOCK === '1';

async function chooseApi(): Promise<Api> {
  const params = new URLSearchParams(window.location.search);
  if (canMock && (window.go === undefined || params.has('mock'))) {
    const { createMockApi, mockSetupFrom } = await import('./mock');
    return createMockApi(mockSetupFrom(window.location.search));
  }
  return createWailsApi();
}

function render(api: Api): void {
  createRoot(document.getElementById('root')!).render(
    <StrictMode>
      <Root api={api} />
    </StrictMode>,
  );
}

// If the mock fails to load (or the Wails bindings throw), fall back to the
// real backend rather than leaving the window blank; if that throws too, show
// the error instead.
chooseApi()
  .catch((err: unknown) => {
    console.error('could not set up the API, falling back to the Go backend:', err);
    return createWailsApi();
  })
  .then(render)
  .catch((err: unknown) => {
    console.error('could not start the app:', err);
    document.getElementById('root')!.textContent = `Could not start the app: ${String(err)}`;
  });
