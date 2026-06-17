import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import { resolve } from 'path';
import { fileURLToPath } from 'url';

const __dirname = fileURLToPath(new URL('.', import.meta.url));

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/__tests__/setup.ts'],
    environmentOptions: {
      jsdom: { url: 'https://example.com/login' },
    },
  },
  resolve: {
    alias: {
      '@passbubble/shared-ts': resolve(__dirname, '../packages/shared-ts/src/index.ts'),
    },
  },
});
