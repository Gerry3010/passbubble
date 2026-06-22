import { defineConfig, mergeConfig } from 'vite';
import baseConfig from './vite.config.js';

// Safari variant of the main (popup/options/fill-iframe/background) build.
// Identical to Chrome/Firefox — Safari is MV3 with a module service worker —
// only the output directory differs. The native Xcode wrapper is generated
// from dist/safari via `xcrun safari-web-extension-converter` (see safari/).
export default mergeConfig(baseConfig, defineConfig({
  build: {
    outDir: 'dist/safari',
    emptyOutDir: true,
  },
}));
