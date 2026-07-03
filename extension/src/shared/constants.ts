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

export const MessageType = {
  // Auth
  LOGIN: 'LOGIN',
  VERIFY_TOTP: 'VERIFY_TOTP',
  UNLOCK: 'UNLOCK',
  LOCK: 'LOCK',
  LOGOUT: 'LOGOUT',
  GET_SESSION: 'GET_SESSION',
  // PIN quick-unlock (device-local; the PIN is never stored)
  SET_PIN: 'SET_PIN',
  UNLOCK_WITH_PIN: 'UNLOCK_WITH_PIN',
  DISABLE_PIN: 'DISABLE_PIN',
  GET_PIN_STATUS: 'GET_PIN_STATUS',
  // Entries
  SEARCH_ENTRIES: 'SEARCH_ENTRIES',
  LIST_FOLDERS: 'LIST_FOLDERS',
  GET_ENTRY: 'GET_ENTRY',
  // Bulk-decrypted usernames ({ [id]: username }) so the popup can search by
  // username without decrypting every entry's full data on demand.
  GET_USERNAMES: 'GET_USERNAMES',
  CREATE_ENTRY: 'CREATE_ENTRY',
  UPDATE_ENTRY: 'UPDATE_ENTRY',
  DELETE_ENTRY: 'DELETE_ENTRY',
  // Autofill
  GET_MATCHES_FOR_URL: 'GET_MATCHES_FOR_URL',
  FILL_ENTRY: 'FILL_ENTRY',
  // Current TOTP code for the entry relevant to a URL (prefers the entry that
  // was just filled in the sender's tab). The secret never leaves the background
  // — only the short-lived code and its remaining lifetime are returned.
  GET_TOTP_FOR_URL: 'GET_TOTP_FOR_URL',
  // Typed autofill (credit cards / identities): list entries of a type with a
  // non-secret display hint, and fetch one entry's decrypted field map to fill
  // a checkout/address form.
  GET_ENTRIES_BY_TYPE: 'GET_ENTRIES_BY_TYPE',
  FILL_TYPED_ENTRY: 'FILL_TYPED_ENTRY',
  // Content script reports the host of the frame that has the login form (e.g. an
  // SSO iframe); the popup queries it to pre-fill search + the "+ Site" toggle.
  REPORT_LOGIN_FRAME: 'REPORT_LOGIN_FRAME',
  GET_FILL_HOST: 'GET_FILL_HOST',
  // Open the extension's toolbar popup (e.g. from the in-page unlock prompt).
  OPEN_POPUP: 'OPEN_POPUP',
  // "Sign in with …" memory (device-local, per host)
  SSO_CANDIDATE: 'SSO_CANDIDATE',
  SSO_GET: 'SSO_GET',
  SSO_DELETE: 'SSO_DELETE',
  // Generator
  GENERATE: 'GENERATE',
  // Save detection
  OFFER_SAVE: 'OFFER_SAVE',
  DISMISS_SAVE: 'DISMISS_SAVE',
  GET_PENDING_SAVE: 'GET_PENDING_SAVE',
  CONFIRM_SAVE: 'CONFIRM_SAVE',
  // Update an existing entry's credentials from a pending save (known site).
  UPDATE_SAVE: 'UPDATE_SAVE',
  // Save blocklist (hosts/domains that never get a "save password?" prompt;
  // autofill still works). Persisted device-locally in storage.local.
  BLOCKLIST_GET: 'BLOCKLIST_GET',
  BLOCKLIST_ADD: 'BLOCKLIST_ADD',
  BLOCKLIST_REMOVE: 'BLOCKLIST_REMOVE',
} as const;

export type MessageType = (typeof MessageType)[keyof typeof MessageType];

export const STORAGE_KEYS = {
  // chrome.storage.sync — persists across devices
  SERVER_URL: 'server_url',
  AUTOFILL_ENABLED: 'autofill_enabled',
  // Idle auto-lock timeout in minutes. 0 = never (lock only on browser close /
  // service-worker eviction). Default applied by readers is AUTO_LOCK_DEFAULT_MINUTES.
  AUTO_LOCK_MINUTES: 'auto_lock_minutes',
  // chrome.storage.local — device-local, persists across browser restarts
  SAVE_BLOCKLIST: 'save_blocklist',
  // Per-host "signs in with <provider>" records ({ [host]: { provider,
  // lastUsed, hits } }). Deliberately not synced — browsing metadata.
  SSO_MEMORY: 'sso_memory',
  // PIN quick-unlock state (chrome.storage.local — survives browser close so the
  // PIN can unlock after a restart). The PIN itself is never stored; PIN_WRAPPED
  // is the master key encrypted under a PIN-derived key. PIN_BOOTSTRAP holds the
  // (already-encrypted) session material needed to rebuild the session, since
  // storage.session is wiped on browser close.
  PIN_ENABLED: 'pin_enabled',
  PIN_SALT: 'pin_salt',
  PIN_WRAPPED: 'pin_wrapped_master_key',
  PIN_KDF_TIME: 'pin_kdf_time',
  PIN_KDF_MEMORY: 'pin_kdf_memory',
  PIN_MAX_TRIES: 'pin_max_tries',
  PIN_FAIL_COUNT: 'pin_fail_count',
  PIN_PW_INTERVAL_DAYS: 'pin_pw_interval_days',
  PIN_LAST_MASTER_UNLOCK: 'pin_last_master_unlock',
  PIN_BOOTSTRAP: 'pin_bootstrap',
  // chrome.storage.session — cleared on browser close
  // Timestamp (ms) of the last vault activity; the auto-lock alarm compares it
  // against AUTO_LOCK_MINUTES to decide when to drop the in-memory session.
  LAST_ACTIVITY: 'last_activity',
  REFRESH_TOKEN: 'refresh_token',
  ENC_PRIV_X25519: 'enc_priv_x25519',
  ENC_PRIV_MLKEM: 'enc_priv_mlkem',
  KDF_SALT: 'kdf_salt',
  KDF_TIME: 'kdf_time',
  KDF_MEMORY: 'kdf_memory',
  USER_ID: 'user_id',
  USER_EMAIL: 'user_email',
  USER_NAME: 'user_name',
  ROLE: 'role',
  PUB_X25519: 'pub_x25519',
  PUB_MLKEM: 'pub_mlkem',
  DISMISSED_SAVE_HOSTS: 'dismissed_save_hosts',
  PENDING_SAVE: 'pending_save',
  // In-progress login draft + 2FA state, so closing the popup mid-login
  // (e.g. to fetch a TOTP code) does not discard what the user already entered.
  AUTH_DRAFT: 'auth_draft',
  PENDING_2FA: 'pending_2fa',
} as const;

/** Default idle auto-lock timeout (minutes) when the user has not chosen one. */
export const AUTO_LOCK_DEFAULT_MINUTES = 15;

/** Name of the recurring chrome.alarms alarm that drives the idle auto-lock. */
export const AUTO_LOCK_ALARM = 'auto-lock';
