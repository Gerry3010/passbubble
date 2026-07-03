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

// Per-site "you sign in here with <provider>" memory. Device-local
// (storage.local): records are written when an OAuth authorization navigation
// is observed — a content-script click candidate alone is only a hint,
// confirmed by the navigation that follows it. Cross-device persistence
// happens via the recorded-hook (sso-entry.ts writes the provider into the
// matching entry's encrypted data); this local store remains the fallback for
// sites without a vault entry.

import browser from 'webextension-polyfill';
import { STORAGE_KEYS } from '../shared/constants.js';
import { providerForUrl, type SsoProvider } from '../shared/sso.js';

export interface SsoRecord {
  provider: SsoProvider;
  lastUsed: number;
  hits: number;
}

type SsoMemory = Record<string, SsoRecord>;

// A click on a "sign in with X" button reported by the content script,
// awaiting confirmation by the OAuth navigation. Keyed by host.
const pendingCandidates = new Map<string, { provider: SsoProvider; ts: number }>();
const CANDIDATE_TTL_MS = 30_000;

// Last non-provider host seen per tab (main frame), and which tab opened a
// tab — OAuth popups are new tabs whose initiating site lives in the opener.
const tabHost = new Map<number, string>();
const tabOpener = new Map<number, number>();

// Pure auth hosts that must never be recorded as the *initiating* site.
// github.com/facebook.com are deliberately absent — signing into GitHub with
// Google is a legitimate record.
const AUTH_ONLY_HOSTS = new Set([
  'accounts.google.com',
  'appleid.apple.com',
  'login.microsoftonline.com',
  'login.live.com',
]);

async function readMemory(): Promise<SsoMemory> {
  try {
    const data = await browser.storage.local.get(STORAGE_KEYS.SSO_MEMORY);
    return (data[STORAGE_KEYS.SSO_MEMORY] as SsoMemory | undefined) ?? {};
  } catch {
    return {};
  }
}

async function writeMemory(memory: SsoMemory): Promise<void> {
  try {
    await browser.storage.local.set({ [STORAGE_KEYS.SSO_MEMORY]: memory });
  } catch {
    // storage unavailable — the memory is a convenience, never fatal
  }
}

export async function getSsoRecord(host: string): Promise<SsoRecord | null> {
  if (!host) return null;
  const memory = await readMemory();
  return memory[host] ?? null;
}

// Invoked after a confirmed SSO use so the provider can also be persisted
// into matching vault entries. Registered by the service worker; absent in
// unit tests, so recording stays side-effect free there.
let onSsoRecorded: ((host: string, provider: SsoProvider) => void) | null = null;

export function setSsoRecordedHook(hook: (host: string, provider: SsoProvider) => void): void {
  onSsoRecorded = hook;
}

export async function recordSsoUse(host: string, provider: SsoProvider): Promise<void> {
  if (!host) return;
  const memory = await readMemory();
  const prev = memory[host];
  memory[host] = {
    provider,
    lastUsed: Date.now(),
    hits: prev?.provider === provider ? prev.hits + 1 : 1,
  };
  await writeMemory(memory);
  onSsoRecorded?.(host, provider);
}

export async function deleteSsoRecord(host: string): Promise<void> {
  const memory = await readMemory();
  if (!(host in memory)) return;
  delete memory[host];
  await writeMemory(memory);
}

/** Content-script hint: a "sign in with X" button was pressed on `host`. */
export function noteSsoCandidate(host: string, provider: SsoProvider): void {
  if (host) pendingCandidates.set(host, { provider, ts: Date.now() });
}

/**
 * Handle a committed main-frame navigation: track per-tab hosts, and when a
 * provider authorization URL is hit, attribute it to the initiating site
 * (same tab's previous host, or the opener tab's for window.open popups) —
 * falling back to a fresh click candidate with the same provider.
 */
export async function handleNavigation(tabId: number, url: string): Promise<void> {
  const provider = providerForUrl(url);
  if (!provider) {
    if (/^https?:/.test(url)) {
      try {
        tabHost.set(tabId, new URL(url).hostname.replace(/^www\./, ''));
      } catch {
        // unparsable URL — keep the previous host
      }
    }
    return;
  }

  let sourceHost = tabHost.get(tabId) ?? '';
  if (!sourceHost) {
    const opener = tabOpener.get(tabId);
    if (opener !== undefined) sourceHost = tabHost.get(opener) ?? '';
  }
  if (!sourceHost) {
    // Unknown source (e.g. popup without tracked opener): fall back to the
    // freshest matching click candidate.
    for (const [host, cand] of pendingCandidates) {
      if (cand.provider === provider && Date.now() - cand.ts <= CANDIDATE_TTL_MS) {
        sourceHost = host;
        break;
      }
    }
  }
  if (!sourceHost || AUTH_ONLY_HOSTS.has(sourceHost)) return;
  pendingCandidates.delete(sourceHost);
  await recordSsoUse(sourceHost, provider);
}

/** Wire the webNavigation/tab listeners. Called once from the service worker. */
export function initSsoMemory(): void {
  const nav = (browser as unknown as { webNavigation?: typeof browser.webNavigation }).webNavigation;
  if (!nav?.onCommitted) return; // permission missing (e.g. stripped build)
  nav.onCommitted.addListener((details) => {
    if (details.frameId !== 0) return; // main frames only
    void handleNavigation(details.tabId, details.url);
  });
  nav.onCreatedNavigationTarget?.addListener((details) => {
    tabOpener.set(details.tabId, details.sourceTabId);
  });
  browser.tabs.onRemoved.addListener((tabId) => {
    tabHost.delete(tabId);
    tabOpener.delete(tabId);
  });
}
