import { defineConfig, mergeConfig } from 'vite';
import baseConfig from './vite.config.content.js';

// Safari variant of the classic-IIFE content-script build (see
// vite.config.content.ts). Only the output directory differs.
export default mergeConfig(baseConfig, defineConfig({
  build: {
    outDir: 'dist/safari',
    emptyOutDir: false,
  },
}));
