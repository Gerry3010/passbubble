// AES-256-GCM using Web Crypto API.
// Wire format matches Go crypto.Encrypt / Decrypt: nonce(12) || ciphertext.

const KEY_ALGO = { name: 'AES-GCM', length: 256 };
const NONCE_LEN = 12;

export async function aesGcmEncrypt(key: Uint8Array, plaintext: Uint8Array): Promise<Uint8Array> {
  const cryptoKey = await crypto.subtle.importKey('raw', key, KEY_ALGO, false, ['encrypt']);
  const nonce = crypto.getRandomValues(new Uint8Array(NONCE_LEN));
  const ct = await crypto.subtle.encrypt({ name: 'AES-GCM', iv: nonce }, cryptoKey, plaintext);
  const out = new Uint8Array(NONCE_LEN + ct.byteLength);
  out.set(nonce);
  out.set(new Uint8Array(ct), NONCE_LEN);
  return out;
}

// Decrypts nonce(12) || ciphertext produced by aesGcmEncrypt / Go Encrypt.
export async function aesGcmDecrypt(key: Uint8Array, nonceAndCiphertext: Uint8Array): Promise<Uint8Array> {
  if (nonceAndCiphertext.length < NONCE_LEN) {
    throw new Error('aes-gcm: ciphertext too short');
  }
  const nonce = nonceAndCiphertext.slice(0, NONCE_LEN);
  const ct = nonceAndCiphertext.slice(NONCE_LEN);
  const cryptoKey = await crypto.subtle.importKey('raw', key, KEY_ALGO, false, ['decrypt']);
  const pt = await crypto.subtle.decrypt({ name: 'AES-GCM', iv: nonce }, cryptoKey, ct);
  return new Uint8Array(pt);
}
