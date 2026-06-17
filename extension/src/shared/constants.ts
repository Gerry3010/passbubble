export const MessageType = {
  // Auth
  LOGIN: 'LOGIN',
  UNLOCK: 'UNLOCK',
  LOCK: 'LOCK',
  GET_SESSION: 'GET_SESSION',
  // Entries
  SEARCH_ENTRIES: 'SEARCH_ENTRIES',
  GET_ENTRY: 'GET_ENTRY',
  CREATE_ENTRY: 'CREATE_ENTRY',
  UPDATE_ENTRY: 'UPDATE_ENTRY',
  DELETE_ENTRY: 'DELETE_ENTRY',
  // Autofill
  GET_MATCHES_FOR_URL: 'GET_MATCHES_FOR_URL',
  FILL_ENTRY: 'FILL_ENTRY',
  // Generator
  GENERATE: 'GENERATE',
  // Save detection
  OFFER_SAVE: 'OFFER_SAVE',
  DISMISS_SAVE: 'DISMISS_SAVE',
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
} as const;
