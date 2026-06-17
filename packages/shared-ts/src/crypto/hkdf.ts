// HKDF-SHA256 using Web Crypto API.

export async function hkdfSha256(
  ikm: Uint8Array,
  salt: Uint8Array | null,
  info: Uint8Array,
  length: number,
): Promise<Uint8Array> {
  const baseKey = await crypto.subtle.importKey('raw', ikm, 'HKDF', false, ['deriveBits']);
  const bits = await crypto.subtle.deriveBits(
    {
      name: 'HKDF',
      hash: 'SHA-256',
      salt: salt ?? new Uint8Array(0),
      info,
    },
    baseKey,
    length * 8,
  );
  return new Uint8Array(bits);
}
