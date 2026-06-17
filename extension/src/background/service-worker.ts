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
import { getSession, setSession } from './session-store.js';
import { buildHandlers } from './message-handler.js';
import { PassbubbleClient } from '@passbubble/shared-ts';
import { STORAGE_KEYS } from '../shared/constants.js';

const handlers = buildHandlers();

browser.runtime.onMessage.addListener((message, _sender) => {
  const { type, payload } = message as { type: string; payload: Record<string, unknown> };
  const handler = handlers[type];
  if (!handler) return;
  // Return the promise directly so the channel stays open
  return handler(payload ?? {});
});

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
