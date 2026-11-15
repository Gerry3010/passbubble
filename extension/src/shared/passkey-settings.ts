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

// Device-local passkey settings (storage.local): whether Passbubble answers
// passkey requests at all, and the sites where it never does. Read by the
// background, edited on the options page.

import browser from 'webextension-polyfill';
import { STORAGE_KEYS } from './constants.js';

export interface PasskeySettings {
  enabled: boolean;
  blocklist: string[];
}

/** A site as the user typed it, reduced to a host: no scheme, path, port or
 * "www.". '' when nothing usable is left. */
export function normalizeSiteEntry(raw: string): string {
  let s = raw.trim().toLowerCase();
  if (!s) return '';
  try {
    s = new URL(s.includes('://') ? s : `https://${s}`).hostname;
  } catch {
    return '';
  }
  return s.replace(/^www\./, '').replace(/\.$/, '');
}

/** True when `host` is a listed site or a subdomain of one. */
export function siteListed(host: string, list: string[]): boolean {
  const h = host.toLowerCase().replace(/^www\./, '').replace(/\.$/, '');
  return list.some((entry) => {
    const e = normalizeSiteEntry(entry);
    return !!e && (h === e || h.endsWith(`.${e}`));
  });
}

export async function readPasskeySettings(): Promise<PasskeySettings> {
  try {
    const s = await browser.storage.local.get([STORAGE_KEYS.PASSKEYS_ENABLED, STORAGE_KEYS.PASSKEY_BLOCKLIST]);
    const list = s[STORAGE_KEYS.PASSKEY_BLOCKLIST];
    return {
      enabled: s[STORAGE_KEYS.PASSKEYS_ENABLED] !== false,
      blocklist: Array.isArray(list) ? list.filter((x): x is string => typeof x === 'string') : [],
    };
  } catch {
    return { enabled: true, blocklist: [] };
  }
}

/** Firefox for Android has no room for the dialog, so the feature is off
 * there. */
export async function isAndroid(): Promise<boolean> {
  try {
    const info = await browser.runtime.getPlatformInfo();
    return info?.os === 'android';
  } catch {
    return false;
  }
}

/** Where the extension cannot answer passkey requests at all, the name of
 * that browser; null where it can. Safari gets no page script (its manifest
 * leaves it out), Firefox for Android no dialog. */
export async function passkeysUnsupportedIn(): Promise<string | null> {
  try {
    if (browser.runtime.getURL('').startsWith('safari-web-extension:')) return 'Safari';
  } catch {
    // no runtime URL: not an extension page
  }
  return (await isAndroid()) ? 'Firefox for Android' : null;
}
