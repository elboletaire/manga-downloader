import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { createWailsApi, type Api } from './api';
import { createMockApi, mockSetupFrom } from './mock';
import { App } from './App';
import '@fontsource-variable/inter/wght.css';
import './styles.css';

// Inside the Go app window.go is set and the real backend answers. In a plain
// browser (pnpm dev) or with ?mock, the mock does.
function chooseApi(): Api {
  const params = new URLSearchParams(window.location.search);
  if (window.go === undefined || params.has('mock')) {
    return createMockApi(mockSetupFrom(window.location.search));
  }

  return createWailsApi();
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App api={chooseApi()} />
  </StrictMode>,
);
