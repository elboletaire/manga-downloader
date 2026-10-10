import react from '@vitejs/plugin-react';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  plugins: [react()],
  server: {
    // the brand's SVGs live at the repo root (assets/brand), outside this app
    fs: { allow: ['../..'] },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  test: {
    include: ['src/**/*.test.ts'],
  },
});
