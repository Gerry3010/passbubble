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
