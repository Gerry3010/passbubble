import { describe, expect, it } from 'vitest';
import { MLKEM768_CT_SIZE, generateMLKEM768, mlkemDecapsulate, mlkemEncapsulate } from '../mlkem.js';

describe('mlkem-768', () => {
  it('ciphertext is exactly 1088 bytes', async () => {
    const { pub } = await generateMLKEM768();
    const { ciphertext } = await mlkemEncapsulate(pub);
    expect(ciphertext.length).toBe(MLKEM768_CT_SIZE);
    expect(MLKEM768_CT_SIZE).toBe(1088);
  });

  it('encapsulate + decapsulate: shared secrets match', async () => {
    const { priv, pub } = await generateMLKEM768();
    const { ciphertext, sharedSecret: ss1 } = await mlkemEncapsulate(pub);
    const ss2 = await mlkemDecapsulate(priv, ciphertext);
    expect(ss1).toEqual(ss2);
  });

  it('wrong private key: shared secrets do NOT match', async () => {
    const { pub } = await generateMLKEM768();
    const { priv: wrongPriv } = await generateMLKEM768();
    const { ciphertext, sharedSecret: ss1 } = await mlkemEncapsulate(pub);
    const ss2 = await mlkemDecapsulate(wrongPriv, ciphertext);
    expect(ss1).not.toEqual(ss2);
  });

  it('different encapsulations produce different shared secrets', async () => {
    const { priv, pub } = await generateMLKEM768();
    const { ciphertext: ct1, sharedSecret: ss1 } = await mlkemEncapsulate(pub);
    const { ciphertext: ct2, sharedSecret: ss2 } = await mlkemEncapsulate(pub);
    expect(ct1).not.toEqual(ct2);
    expect(ss1).not.toEqual(ss2);
    // But both can be decapsulated correctly
    const dec1 = await mlkemDecapsulate(priv, ct1);
    const dec2 = await mlkemDecapsulate(priv, ct2);
    expect(dec1).toEqual(ss1);
    expect(dec2).toEqual(ss2);
  });
});
