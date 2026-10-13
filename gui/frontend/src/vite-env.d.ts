/// <reference types="vite/client" />

// Wails injects window.go with the bound methods; it's absent in a plain browser
interface Window {
  go?: unknown;
}

// Set VITE_MOCK=1 at build time to include the mock API and One Piece dataset
// in a production bundle, e.g. for the screenshot capture build (pass via
// wails.json's frontend:build env or VITE_MOCK=1 wails build).
interface ImportMetaEnv {
  readonly VITE_MOCK?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
