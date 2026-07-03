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

import { describe, it, expect } from 'vitest';
import { base32Decode, generateTotp, parseOtpauthUri } from '../totp.js';

describe('TOTP', () => {
  it('decodes base32 (RFC 4648 "Hello!")', () => {
    // "Hello!" → JBSWY3DPEHPK3PXP is the GitHub demo secret; check a simple one.
    const bytes = base32Decode('JBSWY3DP'); // "Hello"
    expect(new TextDecoder().decode(bytes)).toBe('Hello');
  });

  it('ignores spaces, dashes and lowercase', () => {
    const a = base32Decode('jbsw y3dp');
    const b = base32Decode('JBSW-Y3DP');
    expect(Array.from(a)).toEqual(Array.from(b));
  });

  it('matches the RFC 6238 SHA-1 test vector at T=59s', async () => {
    // RFC 6238 seed "12345678901234567890" in base32, 8 digits, expected 94287082.
    const secretBase32 = 'GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ';
    const { code } = await generateTotp(secretBase32, { digits: 8 }, 59 * 1000);
    expect(code).toBe('94287082');
  });

  it('reports seconds remaining within the period', async () => {
    const { secondsRemaining } = await generateTotp('JBSWY3DPEHPK3PXP', {}, 0);
    expect(secondsRemaining).toBeGreaterThan(0);
    expect(secondsRemaining).toBeLessThanOrEqual(30);
  });
});

describe('parseOtpauthUri', () => {
  it('parses a full otpauth URI', () => {
    const p = parseOtpauthUri(
      'otpauth://totp/GitHub:alice?secret=JBSWY3DPEHPK3PXP&issuer=GitHub&digits=6&period=30&algorithm=SHA1',
    );
    expect(p).toEqual({
      secret: 'JBSWY3DPEHPK3PXP',
      label: 'GitHub:alice',
      issuer: 'GitHub',
      digits: 6,
      period: 30,
      algorithm: 'SHA1',
    });
  });

  it('derives the issuer from the label when the param is missing', () => {
    const p = parseOtpauthUri('otpauth://totp/Example:bob%40example.com?secret=JBSWY3DPEHPK3PXP');
    expect(p?.issuer).toBe('Example');
    expect(p?.label).toBe('Example:bob@example.com');
  });

  it('leaves optional params undefined when absent', () => {
    const p = parseOtpauthUri('otpauth://totp/plain?secret=JBSWY3DPEHPK3PXP');
    expect(p).toMatchObject({ secret: 'JBSWY3DPEHPK3PXP', label: 'plain' });
    expect(p?.digits).toBeUndefined();
    expect(p?.period).toBeUndefined();
  });

  it('rejects hotp URIs', () => {
    expect(parseOtpauthUri('otpauth://hotp/x?secret=JBSWY3DPEHPK3PXP&counter=1')).toBeNull();
  });

  it('rejects a missing or invalid base32 secret', () => {
    expect(parseOtpauthUri('otpauth://totp/x')).toBeNull();
    expect(parseOtpauthUri('otpauth://totp/x?secret=notbase32!!')).toBeNull();
  });

  it('rejects non-otpauth strings', () => {
    expect(parseOtpauthUri('https://example.com')).toBeNull();
    expect(parseOtpauthUri('JBSWY3DPEHPK3PXP')).toBeNull();
  });
});
