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

// Reading an entry's 2FA data. Everything that reads the secret or generates a
// code goes through here.

import { parseOtpauthUri, type TotpOptions } from '../crypto/totp.js';
import type { EntryData } from '../types/vault.js';

/** The entry's 2FA secret, or undefined when it has none. Older TUI builds copied
 * the password into totp_secret on plain password entries, so a value equal to
 * the password counts as absent. The stored data stays as it is. */
export function totpSecretOf(data: Pick<EntryData, 'totp_secret' | 'password'> | null | undefined): string | undefined {
  const secret = data?.totp_secret;
  if (typeof secret !== 'string' || !secret) return undefined;
  return secret === data?.password ? undefined : secret;
}

/** Removes a stray totp_secret (see totpSecretOf), and only a stray one. A writer
 * that is about to change the password has to call this first: the old copy would
 * no longer equal the new password and would then pass for a real secret. */
export function dropStrayTotpSecret(data: EntryData): void {
  if (data.totp_secret && !totpSecretOf(data)) delete data.totp_secret;
}

function positiveInt(raw: unknown): number | undefined {
  const n = typeof raw === 'string' ? Number(raw) : raw;
  return typeof n === 'number' && Number.isInteger(n) && n > 0 ? n : undefined;
}

/** The code parameters stored with an entry or custom field. Values that cannot
 * be used are left out, so generateTotp falls back to its default for them. */
export function totpOptionsOf(source: { period?: unknown; digits?: unknown; algorithm?: unknown } | null | undefined): TotpOptions {
  const options: TotpOptions = {};
  const period = positiveInt(source?.period);
  const digits = positiveInt(source?.digits);
  if (period) options.period = period;
  if (digits) options.digits = digits;
  if (typeof source?.algorithm === 'string' && source.algorithm) options.algorithm = source.algorithm;
  return options;
}

/** What it takes to generate a code from a custom `totp` field, or null when it
 * is empty. Its value is an otpauth:// URI, which defines the code by itself, or
 * a bare secret: then the field's own parameters count first, the entry's next. */
export function customFieldTotp(
  field: { value?: unknown; period?: unknown; digits?: unknown; algorithm?: unknown },
  data: Pick<EntryData, 'period' | 'digits' | 'algorithm'> | null | undefined,
): { secret: string; options: TotpOptions } | null {
  const value = typeof field.value === 'string' ? field.value.trim() : '';
  if (!value) return null;
  const uri = parseOtpauthUri(value);
  if (uri) return { secret: uri.secret, options: totpOptionsOf(uri) };
  return { secret: value, options: { ...totpOptionsOf(data), ...totpOptionsOf(field) } };
}
