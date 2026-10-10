/// <reference types="vite/client" />

// Wails injects window.go with the bound methods; it's absent in a plain browser
interface Window {
  go?: unknown;
}
