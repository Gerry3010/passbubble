// Intercepts form submissions to offer saving new credentials.
// Does NOT call preventDefault() — the save offer is non-blocking.

import browser from 'webextension-polyfill';
import { MessageType, STORAGE_KEYS } from '../shared/constants.js';
import { normaliseHost } from '../shared/utils.js';

interface DetectedCredentials {
  username: string;
  password: string;
}

function extractCredentials(form: HTMLFormElement): DetectedCredentials | null {
  const pw = form.querySelector<HTMLInputElement>('input[type="password"]');
  if (!pw || !pw.value) return null;
  const username =
    form.querySelector<HTMLInputElement>(
      'input[type="email"], input[type="text"], input[autocomplete="username"]',
    )?.value ?? '';
  return { username, password: pw.value };
}

export function initSaveDetector(): void {
  document.addEventListener(
    'submit',
    async (e) => {
      const form = e.target as HTMLFormElement;
      const creds = extractCredentials(form);
      if (!creds) return;

      const host = normaliseHost(location.href);

      // Check if user dismissed saves for this host
      const stored = await browser.storage.session.get(STORAGE_KEYS.DISMISSED_SAVE_HOSTS);
      const dismissed = (stored[STORAGE_KEYS.DISMISSED_SAVE_HOSTS] as string[] | undefined) ?? [];
      if (dismissed.includes(host)) return;

      // Check for existing entry (don't offer if URL already matched)
      const matches = await browser.runtime.sendMessage({
        type: MessageType.GET_MATCHES_FOR_URL,
        payload: { url: location.href },
      });
      if (Array.isArray(matches) && matches.length > 0) return;

      await browser.runtime.sendMessage({
        type: MessageType.OFFER_SAVE,
        payload: {
          name: document.title || host,
          url: location.href,
          username: creds.username,
          password: creds.password,
        },
      });
    },
    true, // capture phase — collect values before form handlers clear them
  );
}
