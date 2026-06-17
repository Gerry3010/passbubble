import { describe, expect, it } from 'vitest';
import { hkdfSha256 } from '../hkdf.js';

// RFC 5869 Test Case 1
// https://datatracker.ietf.org/doc/html/rfc5869#appendix-A.1
describe('hkdf-sha256', () => {
  it('matches RFC 5869 test case 1', async () => {
    const ikm = new Uint8Array(22).fill(0x0b);
    const salt = new Uint8Array([
      0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c,
    ]);
    const info = new Uint8Array([
      0xf0, 0xf1, 0xf2, 0xf3, 0xf4, 0xf5, 0xf6, 0xf7, 0xf8, 0xf9,
    ]);
    const expected =
      '3cb25f25faacd57a90434f64d0362f2a2d2d0a90cf1a5a4c5db02d56ecc4c5bf' +
      '34007208d5b887185865';
    const result = await hkdfSha256(ikm, salt, info, 42);
    expect(Buffer.from(result).toString('hex')).toBe(expected);
  });

  it('null salt uses empty salt', async () => {
    const ikm = new Uint8Array(32).fill(0xcc);
    const info = new TextEncoder().encode('test-info');
    const r1 = await hkdfSha256(ikm, null, info, 32);
    const r2 = await hkdfSha256(ikm, new Uint8Array(0), info, 32);
    expect(r1).toEqual(r2);
  });

  it('different IKM produces different output', async () => {
    const info = new TextEncoder().encode('passbubble-hybrid-kem-v1');
    const r1 = await hkdfSha256(new Uint8Array(32).fill(0x01), null, info, 32);
    const r2 = await hkdfSha256(new Uint8Array(32).fill(0x02), null, info, 32);
    expect(r1).not.toEqual(r2);
  });
});
