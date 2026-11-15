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

interface ContentScript {
  matches: string[];
  js: string[];
  run_at?: string;
  world?: string;
  all_frames?: boolean;
}

function manifest(browser: string) {
  return JSON.parse(MANIFESTS[browser]) as {
    background: { service_worker?: string; scripts?: string[]; type?: string };
    content_security_policy: unknown;
    content_scripts: ContentScript[];
    web_accessible_resources: Array<{ resources: string[]; matches: string[] }>;
  };
}

const scriptWith = (browser: string, file: string) => manifest(browser).content_scripts.find((c) => c.js.includes(file));
const accessible = (browser: string) => manifest(browser).web_accessible_resources.flatMap((w) => w.resources);

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

  describe('passkeys', () => {
    it.each(['chrome', 'firefox'])('%s injects the page script into the MAIN world of the top frame at document_start', (browser) => {
      expect(scriptWith(browser, 'content/webauthn-page.js')).toEqual({
        matches: ['<all_urls>'],
        js: ['content/webauthn-page.js'],
        run_at: 'document_start',
        world: 'MAIN',
        all_frames: false,
      });
    });

    it.each(['chrome', 'firefox'])('%s runs the bridge in the isolated world of the top frame at document_start', (browser) => {
      const bridge = scriptWith(browser, 'content/webauthn-bridge.js');
      expect(bridge).toEqual({
        matches: ['<all_urls>'],
        js: ['content/webauthn-bridge.js'],
        run_at: 'document_start',
        all_frames: false,
      });
      expect(bridge?.world).toBeUndefined();
    });

    it.each(['chrome', 'firefox'])('%s lets pages embed the passkey dialog', (browser) => {
      expect(accessible(browser)).toContain('passkey-dialog/index.html');
    });

    it('Safari has none of it', () => {
      expect(scriptWith('safari', 'content/webauthn-page.js')).toBeUndefined();
      expect(scriptWith('safari', 'content/webauthn-bridge.js')).toBeUndefined();
      expect(accessible('safari')).not.toContain('passkey-dialog/index.html');
    });

    it.each(['chrome', 'firefox', 'safari'])('%s keeps the main content script in every frame', (browser) => {
      expect(scriptWith(browser, 'content/content-script.js')).toMatchObject({ run_at: 'document_idle', all_frames: true });
    });
  });
});
