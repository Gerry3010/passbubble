import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { resolve } from 'path';

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist/chrome',
    emptyOutDir: true,
    rollupOptions: {
      input: {
        'popup/index': resolve(__dirname, 'src/popup/index.html'),
        'options/index': resolve(__dirname, 'src/options/index.html'),
        'fill-iframe/index': resolve(__dirname, 'src/fill-iframe/index.html'),
        'background/service-worker': resolve(__dirname, 'src/background/service-worker.ts'),
        // NOTE: the content script is built separately (vite.config.content.ts)
        // as a classic IIFE — MV3 injects content_scripts as non-module scripts,
        // so it must not contain ESM `import` statements.
      },
      output: {
        entryFileNames: '[name].js',
        chunkFileNames: 'chunks/[name]-[hash].js',
        assetFileNames: 'assets/[name].[ext]',
      },
    },
  },
  resolve: {
    alias: {
      '@shared': resolve(__dirname, '../packages/shared-ts/src'),
      '@passbubble/shared-ts': resolve(__dirname, '../packages/shared-ts/src/index.ts'),
    },
  },
});
