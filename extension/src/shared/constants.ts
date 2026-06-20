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
  // Entries
  SEARCH_ENTRIES: 'SEARCH_ENTRIES',
  LIST_FOLDERS: 'LIST_FOLDERS',
  GET_ENTRY: 'GET_ENTRY',
  CREATE_ENTRY: 'CREATE_ENTRY',
  UPDATE_ENTRY: 'UPDATE_ENTRY',
  DELETE_ENTRY: 'DELETE_ENTRY',
  // Autofill
  GET_MATCHES_FOR_URL: 'GET_MATCHES_FOR_URL',
  FILL_ENTRY: 'FILL_ENTRY',
  // Content script reports the host of the frame that has the login form (e.g. an
  // SSO iframe); the popup queries it to pre-fill search + the "+ Site" toggle.
  REPORT_LOGIN_FRAME: 'REPORT_LOGIN_FRAME',
  GET_FILL_HOST: 'GET_FILL_HOST',
  // Generator
  GENERATE: 'GENERATE',
  // Save detection
  OFFER_SAVE: 'OFFER_SAVE',
  DISMISS_SAVE: 'DISMISS_SAVE',
  GET_PENDING_SAVE: 'GET_PENDING_SAVE',
  CONFIRM_SAVE: 'CONFIRM_SAVE',
} as const;

export type MessageType = (typeof MessageType)[keyof typeof MessageType];

export const STORAGE_KEYS = {
  // chrome.storage.sync — persists across devices
  SERVER_URL: 'server_url',
  AUTOFILL_ENABLED: 'autofill_enabled',
  // chrome.storage.session — cleared on browser close
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
