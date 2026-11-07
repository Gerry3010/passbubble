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

/// <reference types="vite/client" />

import { describe, it, expect } from 'vitest';
import chrome from '../../manifest.chrome.json?raw';
import firefox from '../../manifest.firefox.json?raw';
import safari from '../../manifest.safari.json?raw';

const CSP = "script-src 'self' 'wasm-unsafe-eval'; object-src 'self';";
const BUNDLE = 'background/service-worker.js';
const MANIFESTS: Record<string, string> = { chrome, firefox, safari };

function manifest(browser: string) {
  return JSON.parse(MANIFESTS[browser]) as {
    background: { service_worker?: string; scripts?: string[]; type?: string };
    content_security_policy: unknown;
  };
}

describe('manifests', () => {
  // Firefox refuses to install an add-on with only background.service_worker.
  it('Firefox runs the background as an event page', () => {
    const { background } = manifest('firefox');
    expect(background.scripts).toEqual([BUNDLE]);
    expect(background.service_worker).toBeUndefined();
    // The bundle imports its chunks, so it has to load as a module.
    expect(background.type).toBe('module');
  });

  it.each(['chrome', 'safari'])('%s runs the background as a service worker', (browser) => {
    expect(manifest(browser).background).toEqual({ service_worker: BUNDLE, type: 'module' });
  });

  it.each(['firefox', 'chrome', 'safari'])('%s uses the MV3 object form of the CSP, allowing WASM', (browser) => {
    expect(manifest(browser).content_security_policy).toEqual({ extension_pages: CSP });
  });
});
