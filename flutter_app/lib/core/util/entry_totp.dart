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

// Reading an entry's TOTP secret and producing its codes, from the decrypted
// entry JSON.

import 'package:otp/otp.dart';

/// Key of the TOTP secret inside the decrypted entry JSON.
const totpSecretKey = 'totp_secret';

/// Keys of the options stored next to the secret, see [TotpOptions.stored].
const totpOptionKeys = ['period', 'digits', 'algorithm'];

/// Whether `totp_secret` in [data] is a stray copy of the password. Older TUI
/// builds wrote those on plain password entries; such a value is no 2FA secret.
bool isStrayTotpSecret(Map<String, dynamic> data) {
  final secret = data[totpSecretKey];
  return secret is String && secret.isNotEmpty && secret == data['password'];
}

/// The entry's TOTP secret, or null when it has none. Read the secret through
/// this, so a stray one (see [isStrayTotpSecret]) counts as absent everywhere.
String? totpSecretOf(Map<String, dynamic> data) {
  final secret = data[totpSecretKey];
  if (secret is! String || secret.isEmpty || isStrayTotpSecret(data)) return null;
  return secret;
}

/// Whether [a] and [b] are one TOTP key spelled differently, in case, grouping or
/// padding. An otpauth:// URL brings its own options, so it only equals the same
/// URL, not the bare key inside it. Same rules as `SameSecret` in
/// cli/pkg/totp/totp.go.
bool isSameTotpKey(String a, String b) => _unpadded(a) == _unpadded(b);

String _unpadded(String key) => _bareKey(key).replaceFirst(RegExp(r'=+$'), '');

String _bareKey(String key) => key.replaceAll(RegExp(r'[\s_-]'), '').toUpperCase();

/// How an entry's codes are computed. Unset or unknown values mean the defaults
/// (same rules as `StoredOptions` in cli/pkg/totp/totp.go).
class TotpOptions {
  final int period;
  final int digits;
  final Algorithm algorithm;

  const TotpOptions({
    this.period = 30,
    this.digits = 6,
    this.algorithm = Algorithm.SHA1,
  });

  /// The options stored in the decrypted entry JSON. They apply to the entry's
  /// `totp_secret` and to its custom `totp` fields alike.
  factory TotpOptions.stored(Map<String, dynamic> data) =>
      TotpOptions._parse(data['period'], data['digits'], data['algorithm']);

  factory TotpOptions._parse(Object? period, Object? digits, Object? algorithm) {
    final p = _intOf(period);
    return TotpOptions(
      period: p != null && p > 0 ? p : 30,
      digits: _intOf(digits) == 8 ? 8 : 6,
      algorithm: algorithm is String
          ? switch (algorithm.trim().toUpperCase()) {
              'SHA256' => Algorithm.SHA256,
              'SHA512' => Algorithm.SHA512,
              _ => Algorithm.SHA1,
            }
          : Algorithm.SHA1,
    );
  }

  static int? _intOf(Object? v) => switch (v) {
        final num n => n.toInt(),
        final String s => int.tryParse(s.trim()),
        _ => null,
      };
}

/// The code for [secret] at [timeMs] and the seconds it stays valid, or null
/// when [secret] cannot produce one. [secret] may be a base32 secret, grouped or
/// lowercase, or a whole otpauth:// URL as importers store it, whose parameters
/// then override [stored].
({String code, int remaining})? totpCodeAt(
  String secret,
  TotpOptions stored,
  int timeMs,
) {
  try {
    var key = secret.trim();
    var options = stored;
    if (key.toLowerCase().startsWith('otpauth://')) {
      final uri = Uri.parse(key);
      final q = uri.queryParameters;
      key = q['secret'] ?? '';
      options = TotpOptions._parse(q['period'], q['digits'], q['algorithm']);
    }
    key = _bareKey(key);
    // An empty key would produce a valid-looking code.
    if (key.isEmpty) return null;
    final code = OTP.generateTOTPCodeString(
      key,
      timeMs,
      length: options.digits,
      interval: options.period,
      algorithm: options.algorithm,
      isGoogle: true,
    );
    return (
      code: code,
      remaining: options.period - (timeMs ~/ 1000 % options.period),
    );
  } catch (_) {
    return null; // not base32, or a broken URL
  }
}
