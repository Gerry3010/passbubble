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

// HTTP Basic Auth autofill: answer the browser-native 401 credential dialog with
// a matching vault entry. Chrome/Firefox only — Safari does not expose
// webRequest.onAuthRequired. The credential lookup itself lives in
// message-handler (resolveBasicAuthCredentials); this module is just the
// browser-specific webRequest plumbing.

import browser from 'webextension-polyfill';
import { resolveBasicAuthCredentials } from './message-handler.js';

interface AuthDetails {
  requestId: string;
  url: string;
  isProxy?: boolean;
}

type BlockingResponse = { authCredentials?: { username: string; password: string } } | Record<string, never>;

// Requests we have already answered once, so a wrong password does not cause an
// infinite re-challenge loop — on the second prompt for the same request we step
// aside and let the browser show its native dialog. Cleared when the request
// finishes (onCompleted) or fails (onErrorOccurred).
const answered = new Set<string>();

async function handle(details: AuthDetails): Promise<BlockingResponse> {
  // Server auth only (not proxies), and only once per request.
  if (details.isProxy) return {};
  if (answered.has(details.requestId)) return {};
  const creds = await resolveBasicAuthCredentials(details.url);
  if (!creds) return {};
  answered.add(details.requestId);
  return { authCredentials: creds };
}

// Firefox exposes runtime.getBrowserInfo; Chrome does not. The two browsers also
// differ in how onAuthRequired handles an async answer: Firefox accepts a
// returned Promise with ['blocking'], Chrome MV3 requires ['asyncBlocking'] plus
// an explicit callback.
function isFirefox(): boolean {
  return typeof (browser as { runtime?: { getBrowserInfo?: unknown } }).runtime?.getBrowserInfo === 'function';
}

let registered = false;

export function registerBasicAuthHandler(): void {
  if (registered) return;
  const wr = (globalThis as { chrome?: typeof chrome }).chrome?.webRequest;
  // No webRequest (e.g. Safari) → basic-auth autofill is simply unavailable.
  if (!wr?.onAuthRequired) return;
  registered = true;

  const filter = { urls: ['<all_urls>'] };
  if (isFirefox()) {
    // webextension-polyfill's typings cover the Promise-returning Firefox form.
    browser.webRequest.onAuthRequired.addListener(
      (details) => handle(details as unknown as AuthDetails) as Promise<browser.WebRequest.BlockingResponse>,
      filter,
      ['blocking'],
    );
  } else {
    wr.onAuthRequired.addListener(
      (details, callback) => {
        void handle(details as unknown as AuthDetails).then((r) => callback?.(r as chrome.webRequest.BlockingResponse));
      },
      filter,
      ['asyncBlocking'],
    );
  }

  const forget = (d: { requestId: string }) => answered.delete(d.requestId);
  wr.onCompleted.addListener(forget, filter);
  wr.onErrorOccurred.addListener(forget, filter);
}
