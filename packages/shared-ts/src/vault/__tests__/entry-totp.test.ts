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
import { customFieldTotp, dropStrayTotpSecret, totpOptionsOf, totpSecretOf } from '../entry-totp.js';

describe('totpSecretOf', () => {
  it('returns a real secret', () => {
    expect(totpSecretOf({ password: 'hunter2', totp_secret: 'JBSWY3DP' })).toBe('JBSWY3DP');
    expect(totpSecretOf({ totp_secret: 'JBSWY3DP' })).toBe('JBSWY3DP');
  });

  it('treats a copy of the password as absent', () => {
    expect(totpSecretOf({ password: 'JBSWY3DP', totp_secret: 'JBSWY3DP' })).toBeUndefined();
  });

  it('is undefined without a usable secret', () => {
    expect(totpSecretOf({ password: 'x' })).toBeUndefined();
    expect(totpSecretOf({ password: '', totp_secret: '' })).toBeUndefined();
    expect(totpSecretOf(null)).toBeUndefined();
    expect(totpSecretOf({ totp_secret: 42 as unknown as string })).toBeUndefined();
  });
});

describe('dropStrayTotpSecret', () => {
  it('removes the stray copy', () => {
    const data = { password: 'p', totp_secret: 'p', notes: 'n' };
    dropStrayTotpSecret(data);
    expect(data).toEqual({ password: 'p', notes: 'n' });
  });

  it('leaves a real secret untouched', () => {
    const data = { password: 'p', totp_secret: 'JBSWY3DP' };
    dropStrayTotpSecret(data);
    expect(data).toEqual({ password: 'p', totp_secret: 'JBSWY3DP' });
  });
});

describe('totpOptionsOf', () => {
  it('reads the stored parameters', () => {
    expect(totpOptionsOf({ period: 60, digits: 8, algorithm: 'SHA256' })).toEqual({
      period: 60,
      digits: 8,
      algorithm: 'SHA256',
    });
    expect(totpOptionsOf({ period: '45', digits: '7' })).toEqual({ period: 45, digits: 7 });
  });

  it('leaves out what is missing or cannot be used', () => {
    expect(totpOptionsOf({})).toEqual({});
    expect(totpOptionsOf(null)).toEqual({});
    expect(totpOptionsOf({ period: 0, digits: -6, algorithm: '' })).toEqual({});
    expect(totpOptionsOf({ period: 'soon', digits: 6.5, algorithm: 1 })).toEqual({});
  });
});

describe('customFieldTotp', () => {
  const entry = { period: 60, digits: 8, algorithm: 'SHA256' };

  it('uses the entry parameters for a bare secret', () => {
    expect(customFieldTotp({ value: ' JBSWY3DP ' }, entry)).toEqual({ secret: 'JBSWY3DP', options: entry });
  });

  it("prefers the field's own parameters", () => {
    expect(customFieldTotp({ value: 'JBSWY3DP', digits: 6 }, entry)?.options).toEqual({ ...entry, digits: 6 });
  });

  it('takes everything from an otpauth URI', () => {
    expect(customFieldTotp({ value: 'otpauth://totp/x?secret=MFRGGZDF&period=45' }, entry)).toEqual({
      secret: 'MFRGGZDF',
      options: { period: 45 },
    });
  });

  it('is null for an empty field', () => {
    expect(customFieldTotp({ value: '  ' }, entry)).toBeNull();
    expect(customFieldTotp({}, entry)).toBeNull();
  });
});
