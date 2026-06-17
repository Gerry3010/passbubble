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
