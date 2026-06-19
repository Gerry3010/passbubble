import { defineConfig } from 'vite';
import { resolve } from 'path';

// Dedicated build for the content script.
//
// MV3 injects entries listed under `content_scripts[].js` as CLASSIC scripts,
// not ES modules — so the output must be a single self-contained file with no
// `import` statements (otherwise the browser throws "Cannot use import
// statement outside a module" and the whole content script fails to run).
//
// The main build (vite.config.ts) emits ESM for popup/options/background, which
// are loaded as modules and may share code chunks. The content script can't, so
// it's bundled here as an IIFE with every dependency inlined.
export default defineConfig({
  build: {
    outDir: 'dist/chrome',
    emptyOutDir: false, // runs after the main build — don't wipe its output
    rollupOptions: {
      input: {
        'content/content-script': resolve(__dirname, 'src/content/content-script.ts'),
      },
      output: {
        entryFileNames: '[name].js',
        format: 'iife',
        inlineDynamicImports: true,
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
