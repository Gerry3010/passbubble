// Copyright (C) 2026 Gerald Hofbauer <info@geraldhofbauer.net>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

import { defineConfig } from 'vite';
import { resolve } from 'path';

// Page script for passkeys (content/webauthn-page.js). The manifest injects it
// into the MAIN world at document_start (Chrome and Firefox, not Safari), so
// like the content script it must be one classic script without `import`.
// It runs among the page's own scripts: no webextension-polyfill, no shared
// chunks — only src/webauthn/protocol.ts and src/content/webauthn-*.ts.
export default defineConfig({
  build: {
    outDir: 'dist/chrome',
    emptyOutDir: false, // runs after the main build — don't wipe its output
    rollupOptions: {
      input: {
        'content/webauthn-page': resolve(__dirname, 'src/content/webauthn-page.ts'),
      },
      output: {
        entryFileNames: '[name].js',
        format: 'iife',
        inlineDynamicImports: true,
      },
    },
  },
});
