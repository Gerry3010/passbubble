import { describe, expect, it } from 'vitest';
import { MLKEM768_CT_SIZE, generateMLKEM768 } from '../mlkem.js';
import { generateX25519 } from '../x25519.js';
import { decryptDataKey, encryptDataKey } from '../hybrid-kem.js';

const X25519_PUB_LEN = 32;

describe('hybrid-kem', () => {
  it('encrypt + decrypt round-trip: recovered key matches original', async () => {
    const { priv: privX, pub: pubX } = generateX25519();
    const { priv: privM, pub: pubM } = await generateMLKEM768();
    const dataKey = crypto.getRandomValues(new Uint8Array(32));

    const encKey = await encryptDataKey(dataKey, pubX, pubM);
    const recovered = await decryptDataKey(encKey, privX, privM);

    expect(recovered).toEqual(dataKey);
  });

  it('wire format: ephemPub(32) || mlkemCT(1088) || gcm output', async () => {
    const { pub: pubX } = generateX25519();
    const { pub: pubM } = await generateMLKEM768();
    const dataKey = crypto.getRandomValues(new Uint8Array(32));

    const encKey = await encryptDataKey(dataKey, pubX, pubM);

    expect(encKey.length).toBeGreaterThan(X25519_PUB_LEN + MLKEM768_CT_SIZE + 12 + 16);
    // Offsets: 0-31 = x25519 ephemeral pub, 32-1119 = ml-kem ct, 1120+ = aes-gcm output
    expect(encKey.length - X25519_PUB_LEN - MLKEM768_CT_SIZE).toBe(
      12 + 32 + 16, // nonce(12) + key(32) + tag(16)
    );
  });

  it('wrong x25519 key: decrypt throws', async () => {
    const { pub: pubX } = generateX25519();
    const { priv: wrongPrivX } = generateX25519();
    const { priv: privM, pub: pubM } = await generateMLKEM768();
    const dataKey = crypto.getRandomValues(new Uint8Array(32));

    const encKey = await encryptDataKey(dataKey, pubX, pubM);
    await expect(decryptDataKey(encKey, wrongPrivX, privM)).rejects.toThrow();
  });

  it('truncated encKey throws', async () => {
    const { priv: privX } = generateX25519();
    const { priv: privM } = await generateMLKEM768();
    await expect(decryptDataKey(new Uint8Array(32), privX, privM)).rejects.toThrow();
  });

  it('two encryptions of same key produce different blobs', async () => {
    const { pub: pubX } = generateX25519();
    const { pub: pubM } = await generateMLKEM768();
    const dataKey = crypto.getRandomValues(new Uint8Array(32));

    const enc1 = await encryptDataKey(dataKey, pubX, pubM);
    const enc2 = await encryptDataKey(dataKey, pubX, pubM);
    expect(enc1).not.toEqual(enc2);
  });
});
