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

// Bridge content script for passkeys (content/webauthn-bridge.js): isolated
// world, top frame, document_start, Chrome and Firefox. A second classic
// script next to the main content script, so it needs its own single-input
// IIFE build (see vite.config.content.ts for why).
export default defineConfig({
  build: {
    outDir: 'dist/chrome',
    emptyOutDir: false, // runs after the main build — don't wipe its output
    rollupOptions: {
      input: {
        'content/webauthn-bridge': resolve(__dirname, 'src/content/webauthn-bridge.ts'),
      },
      output: {
        entryFileNames: '[name].js',
        format: 'iife',
        inlineDynamicImports: true,
      },
    },
  },
});
