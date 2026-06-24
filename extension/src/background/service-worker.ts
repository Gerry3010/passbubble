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

// MV3 Background Service Worker — entry point.
// All E2E crypto operations run here; never in content scripts or popup.

import browser from 'webextension-polyfill';
import { getSession, setSession, clearLoginFrameHost } from './session-store.js';
import { buildHandlers } from './message-handler.js';
import { registerBasicAuthHandler } from './basic-auth.js';
import { savePinRefreshToken } from './pin-store.js';
import { PassbubbleClient } from '@passbubble/shared-ts';
import { STORAGE_KEYS } from '../shared/constants.js';

const handlers = buildHandlers();

// HTTP Basic Auth autofill (Chrome/Firefox; no-op where webRequest.onAuthRequired
// is unavailable). Registered top-level so it survives service-worker restarts.
registerBasicAuthHandler();

browser.runtime.onMessage.addListener((message, sender) => {
  const { type, payload } = message as { type: string; payload: Record<string, unknown> };
  const handler = handlers[type];
  if (!handler) return;
  // Return the promise directly so the channel stays open. Normalise rejections
  // to real Error objects — otherwise a non-Error rejection (e.g. a DOMException
  // from a failed AES/ML-KEM op) surfaces to callers as the unhelpful
  // "listener's promise rejected without an Error".
  return handler(payload ?? {}, sender).catch((err: unknown) => {
    if (err instanceof Error) throw err;
    const msg =
      typeof err === 'string'
        ? err
        : (err as { message?: string } | null)?.message || 'Background error';
    throw new Error(msg);
  });
});

// Forget a tab's recorded login-frame host when it navigates away or closes, so
// the popup never pre-fills a stale host.
browser.tabs.onUpdated.addListener((tabId, info) => {
  if (info.status === 'loading') clearLoginFrameHost(tabId);
});
browser.tabs.onRemoved.addListener((tabId) => clearLoginFrameHost(tabId));

// Token refresh alarm
browser.alarms.onAlarm.addListener(async (alarm) => {
  if (alarm.name !== 'token-refresh') return;
  const session = getSession();
  if (!session) return;

  try {
    const client = new PassbubbleClient(session.serverUrl);
    const resp = await client.refresh(session.refreshToken);
    session.accessToken = resp.access_token;
    session.refreshToken = resp.refresh_token;
    session.accessTokenExpiresAt = Date.now() + resp.expires_in * 1000;
    setSession(session);
    await browser.storage.session.set({ [STORAGE_KEYS.REFRESH_TOKEN]: resp.refresh_token });
    // Keep the PIN bootstrap's copy of the (now-rotated) refresh token current, so
    // a PIN unlock after a browser restart / extension reload does not try to
    // refresh with an already-invalidated token.
    await savePinRefreshToken(resp.refresh_token);
    // Re-schedule for next rotation
    await browser.alarms.create('token-refresh', {
      delayInMinutes: (resp.expires_in - 60) / 60,
    });
  } catch {
    // Token refresh failed; clear session so user sees lock screen
    const { clearSession } = await import('./session-store.js');
    clearSession();
  }
});
