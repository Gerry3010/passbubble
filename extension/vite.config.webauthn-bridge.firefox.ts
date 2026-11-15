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

import { defineConfig, mergeConfig } from 'vite';
import baseConfig from './vite.config.webauthn-bridge.js';

// Firefox variant of the passkey bridge build (see
// vite.config.webauthn-bridge.ts). Only the output directory differs.
export default mergeConfig(baseConfig, defineConfig({
  build: {
    outDir: 'dist/firefox',
    emptyOutDir: false,
  },
}));
