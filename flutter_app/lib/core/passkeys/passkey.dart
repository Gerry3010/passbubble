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

// Passkey records as docs/passkeys.md stores them in the decrypted data of a
// `password` entry, plus the encoding helpers every writer needs: base64url
// without padding, and the P-256 private key in its long PKCS#8 form.

import 'dart:convert';
import 'dart:typed_data';

import 'package:pointycastle/ecc/curves/secp256r1.dart';

/// Key of the passkey array in the decrypted entry data.
const passkeysKey = 'passkeys';

/// COSE algorithm id of ES256, the only one Passbubble handles.
const es256 = -7;

/// The keys [Passkey] reads and writes. Everything else in a record is kept.
const _knownKeys = {
  'credential_id',
  'rp_id',
  'rp_name',
  'user_handle',
  'user_name',
  'user_display_name',
  'key_algorithm',
  'private_key',
  'sign_count',
  'discoverable',
  'prf_secret',
  'created_at',
};

/// One passkey record. [extra] holds the keys this client does not know, which
/// [toJson] writes back unchanged.
class Passkey {
  final String credentialId;
  final String rpId;
  final String rpName;
  final String userHandle;
  final String userName;
  final String userDisplayName;
  final int keyAlgorithm;
  final String privateKey;
  final int signCount;
  final bool discoverable;
  final String? prfSecret;
  final String createdAt;
  final Map<String, dynamic> extra;

  const Passkey({
    required this.credentialId,
    required this.rpId,
    this.rpName = '',
    required this.userHandle,
    this.userName = '',
    this.userDisplayName = '',
    this.keyAlgorithm = es256,
    required this.privateKey,
    this.signCount = 0,
    this.discoverable = true,
    this.prfSecret,
    this.createdAt = '',
    this.extra = const {},
  });

  /// Reads a stored record. Ids in standard base64 or with padding come out as
  /// base64url without padding; a value that does not decode stays as it is.
  factory Passkey.fromJson(Map<String, dynamic> m) {
    String s(String k) => switch (m[k]) {
          final String v => v,
          null => '',
          final v => '$v',
        };
    return Passkey(
      credentialId: _normalizedOrRaw(s('credential_id')),
      rpId: s('rp_id'),
      rpName: s('rp_name'),
      userHandle: _normalizedOrRaw(s('user_handle')),
      userName: s('user_name'),
      userDisplayName: s('user_display_name'),
      keyAlgorithm: _intOf(m['key_algorithm']) ?? es256,
      privateKey: s('private_key'),
      signCount: _intOf(m['sign_count']) ?? 0,
      discoverable: switch (m['discoverable']) {
        final bool b => b,
        final String v => v.trim().toLowerCase() != 'false',
        _ => true,
      },
      prfSecret: m['prf_secret'] is String ? m['prf_secret'] as String : null,
      createdAt: s('created_at'),
      extra: {
        for (final e in m.entries)
          if (!_knownKeys.contains(e.key)) e.key: e.value,
      },
    );
  }

  Map<String, dynamic> toJson() => {
        ...extra,
        'credential_id': credentialId,
        'rp_id': rpId,
        if (rpName.isNotEmpty) 'rp_name': rpName,
        'user_handle': userHandle,
        'user_name': userName,
        if (userDisplayName.isNotEmpty) 'user_display_name': userDisplayName,
        'key_algorithm': keyAlgorithm,
        'private_key': privateKey,
        'sign_count': signCount,
        'discoverable': discoverable,
        'prf_secret': ?prfSecret,
        if (createdAt.isNotEmpty) 'created_at': createdAt,
      };

  /// When the passkey was added, in local time, or null when unknown.
  DateTime? get created => DateTime.tryParse(createdAt)?.toLocal();
}

int? _intOf(Object? v) => switch (v) {
      final num n => n.toInt(),
      final String s => int.tryParse(s.trim()),
      _ => null,
    };

String _normalizedOrRaw(String v) {
  try {
    return v.isEmpty ? v : normalizeBase64Url(v);
  } on FormatException {
    return v;
  }
}

/// The passkey records in decrypted entry [data], as stored: the maps, in their
/// order, without anything that is not a map.
List<Map<String, dynamic>> passkeyRecordsOf(Map<String, dynamic> data) {
  final raw = data[passkeysKey];
  if (raw is! List) return const [];
  return [
    for (final r in raw)
      if (r is Map) r.cast<String, dynamic>(),
  ];
}

/// The passkeys in decrypted entry [data].
List<Passkey> passkeysOf(Map<String, dynamic> data) =>
    passkeyRecordsOf(data).map(Passkey.fromJson).toList();

/// Whether [data] holds at least one passkey record.
bool hasPasskeys(Map<String, dynamic> data) => passkeyRecordsOf(data).isNotEmpty;

/// [data] without its passkeys, for share links: the share viewer cannot show
/// or use a passkey, so its private key stays out of the link.
Map<String, dynamic> withoutPasskeys(Map<String, dynamic> data) =>
    Map.of(data)..remove(passkeysKey);

/// The credential id of a stored [record], normalised, or '' without one.
String credentialIdOf(Map<String, dynamic> record) {
  final v = record['credential_id'];
  return v is String ? _normalizedOrRaw(v) : '';
}

/// RFC 3339 in UTC without fractional seconds, the form `created_at` uses.
String rfc3339Utc(DateTime t) =>
    '${t.toUtc().toIso8601String().split('.').first.replaceAll('Z', '')}Z';

// ── base64url ─────────────────────────────────────────────────────────────────

/// [bytes] as base64url without padding.
String base64UrlNoPad(List<int> bytes) => base64Url.encode(bytes).replaceAll('=', '');

/// Decodes base64url or standard base64, with or without padding and with any
/// whitespace in between. Throws [FormatException] for anything else.
Uint8List decodeBase64Flexible(String input) {
  var s = input.replaceAll(RegExp(r'\s'), '');
  if (!RegExp(r'^[A-Za-z0-9+/_-]*={0,2}$').hasMatch(s)) {
    throw const FormatException('not base64');
  }
  s = s.replaceAll('=', '').replaceAll('-', '+').replaceAll('_', '/');
  if (s.length % 4 == 1) throw const FormatException('not base64');
  s = s.padRight(s.length + (4 - s.length % 4) % 4, '=');
  return base64.decode(s);
}

/// [input] (base64 or base64url, see [decodeBase64Flexible]) re-encoded as
/// base64url without padding.
String normalizeBase64Url(String input) => base64UrlNoPad(decodeBase64Flexible(input));

// ── Private keys ─────────────────────────────────────────────────────────────

/// Why a private key was refused. [message] is meant for the user.
class PasskeyKeyException implements Exception {
  final String message;
  const PasskeyKeyException(this.message);

  @override
  String toString() => message;
}

const _oidEcPublicKey = '2a8648ce3d0201'; // 1.2.840.10045.2.1
const _oidP256 = '2a8648ce3d030107'; // 1.2.840.10045.3.1.7
const _curveNames = {
  _oidP256: 'P-256',
  '2b81040022': 'P-384',
  '2b81040023': 'P-521',
  '2b8104000a': 'secp256k1',
};
const _algorithmNames = {
  '2a864886f70d010101': 'RSA',
  '2b6570': 'Ed25519',
  '2b6571': 'Ed448',
  '2b656e': 'X25519',
};

/// The prefix of the long PKCS#8 form, up to the private scalar: version 0, the
/// ecPublicKey/P-256 algorithm, and the SEC1 key with version 1.
final _pkcs8Head = _hex('308187020100301306072a8648ce3d020106082a8648ce3d030107'
    '046d306b0201010420');

/// Between the scalar and the public key: `[1]` and the BIT STRING header.
final _pkcs8PubHead = _hex('a144034200');

/// Reads a P-256 private key — PEM (`PRIVATE KEY`, PKCS#8, or `EC PRIVATE KEY`,
/// SEC1) or base64/base64url of the DER — checks it and returns it in the long
/// PKCS#8 form with the public key (138 bytes), as base64url without padding.
/// Throws [PasskeyKeyException] with a message for the user otherwise.
String normalizePrivateKey(String input) {
  final der = _derOf(input.trim());
  final d = _p256Scalar(der);
  final curve = ECCurve_secp256r1();
  final n = curve.n;
  final scalar = _bigIntOf(d.scalar);
  if (scalar == BigInt.zero || scalar >= n) {
    throw const PasskeyKeyException('The private key is out of range for P-256.');
  }
  final q = (curve.G * scalar)!;
  final uncompressed = q.getEncoded(false);
  final given = d.publicKey;
  if (given != null &&
      !_sameBytes(given, uncompressed) &&
      !_sameBytes(given, q.getEncoded(true))) {
    throw const PasskeyKeyException(
        'The public key stored with the private key does not belong to it.');
  }
  final out = BytesBuilder()
    ..add(_pkcs8Head)
    ..add(_leftPad(d.scalar, 32))
    ..add(_pkcs8PubHead)
    ..add(uncompressed);
  return base64UrlNoPad(out.toBytes());
}

/// The DER bytes in [input]: the key block of a PEM text, or decoded base64.
Uint8List _derOf(String input) {
  if (input.isEmpty) throw const PasskeyKeyException('Enter the private key.');
  if (input.contains('-----BEGIN')) {
    final blocks = RegExp(r'-----BEGIN ([A-Z0-9 ]+)-----([\s\S]*?)-----END \1-----')
        .allMatches(input)
        .toList();
    final labels = blocks.map((b) => b.group(1)!).toList();
    if (labels.contains('ENCRYPTED PRIVATE KEY')) {
      throw const PasskeyKeyException(
          'The key is encrypted. Export it without a passphrase.');
    }
    final key = blocks
        .where((b) => b.group(1) == 'PRIVATE KEY' || b.group(1) == 'EC PRIVATE KEY')
        .firstOrNull;
    if (key == null) {
      throw PasskeyKeyException(labels.isEmpty
          ? 'The PEM block is incomplete.'
          : 'Expected a PRIVATE KEY or EC PRIVATE KEY block, found ${labels.join(', ')}.');
    }
    // Headers like "Proc-Type:" would belong to an encrypted key.
    if (key.group(2)!.contains(':')) {
      throw const PasskeyKeyException(
          'The key is encrypted. Export it without a passphrase.');
    }
    try {
      return decodeBase64Flexible(key.group(2)!);
    } on FormatException {
      throw const PasskeyKeyException('The PEM block is not valid base64.');
    }
  }
  try {
    return decodeBase64Flexible(input);
  } on FormatException {
    throw const PasskeyKeyException(
        'Not a key: paste a PEM block or the base64 of the DER.');
  }
}

/// The P-256 scalar and, when present, the public key of a PKCS#8 or SEC1 DER
/// key.
({Uint8List scalar, Uint8List? publicKey}) _p256Scalar(Uint8List der) {
  try {
    final top = _Der(der).single(0x30);
    final first = top.next(0x02);
    if (_bigIntOf(first) == BigInt.zero && top.peek == 0x30) {
      // PKCS#8: version 0, AlgorithmIdentifier, OCTET STRING with the SEC1 key.
      final alg = top.nextReader(0x30);
      final algOid = _hexOf(alg.next(0x06));
      if (algOid != _oidEcPublicKey) {
        final name = _algorithmNames[algOid];
        throw PasskeyKeyException(name == null
            ? 'Not an EC key. Passkeys need a P-256 key.'
            : 'This is an $name key. Passkeys need a P-256 key.');
      }
      String? curve;
      if (alg.hasMore && alg.peek == 0x06) curve = _hexOf(alg.next(0x06));
      final inner = _Der(top.next(0x04)).single(0x30);
      return _sec1(inner, outerCurve: curve);
    }
    if (_bigIntOf(first) == BigInt.one && top.peek == 0x04) {
      return _sec1(top, outerCurve: null, versionRead: true);
    }
    throw const PasskeyKeyException('Not a PKCS#8 or SEC1 private key.');
  } on PasskeyKeyException {
    rethrow;
  } catch (_) {
    throw const PasskeyKeyException('The key data is damaged or not a private key.');
  }
}

/// Reads a SEC1 ECPrivateKey: version 1, the scalar, `[0]` curve, `[1]` public
/// key. [outerCurve] is the curve the PKCS#8 wrapper names.
({Uint8List scalar, Uint8List? publicKey}) _sec1(_Der seq,
    {required String? outerCurve, bool versionRead = false}) {
  if (!versionRead && _bigIntOf(seq.next(0x02)) != BigInt.one) {
    throw const PasskeyKeyException('Unsupported EC private key version.');
  }
  var scalar = seq.next(0x04);
  String? curve = outerCurve;
  Uint8List? publicKey;
  while (seq.hasMore) {
    final tag = seq.peek;
    final value = seq.next(tag);
    if (tag == 0xa0) {
      final oid = _hexOf(_Der(value).single(0x06).rest);
      if (curve != null && curve != oid) {
        throw const PasskeyKeyException('The key names two different curves.');
      }
      curve = oid;
    } else if (tag == 0xa1) {
      final bits = _Der(value).single(0x03).rest;
      if (bits.isEmpty || bits[0] != 0) throw const FormatException('bit string');
      publicKey = bits.sublist(1);
    }
  }
  if (curve == null) {
    throw const PasskeyKeyException('The key does not name its curve.');
  }
  if (curve != _oidP256) {
    final name = _curveNames[curve] ?? 'an unsupported curve';
    throw PasskeyKeyException('This key uses $name. Passkeys need a P-256 key.');
  }
  // A scalar with a leading zero byte too many still means the same number.
  while (scalar.length > 32 && scalar[0] == 0) {
    scalar = scalar.sublist(1);
  }
  if (scalar.isEmpty || scalar.length > 32) {
    throw const PasskeyKeyException('The private key has the wrong length for P-256.');
  }
  return (scalar: scalar, publicKey: publicKey);
}

/// A strict reader for definite-length DER.
class _Der {
  final Uint8List _b;
  int _pos = 0;
  _Der(this._b);

  bool get hasMore => _pos < _b.length;
  int get peek => _b[_pos];

  /// The value bytes of the next element, which must have [tag].
  Uint8List next(int tag) {
    if (!hasMore || _b[_pos] != tag) throw const FormatException('unexpected tag');
    _pos++;
    var len = _b[_pos++];
    if (len & 0x80 != 0) {
      final count = len & 0x7f;
      if (count == 0 || count > 3) throw const FormatException('bad length');
      len = 0;
      for (var i = 0; i < count; i++) {
        len = (len << 8) | _b[_pos++];
      }
    }
    if (_pos + len > _b.length) throw const FormatException('truncated');
    final v = Uint8List.sublistView(_b, _pos, _pos + len);
    _pos += len;
    return v;
  }

  _Der nextReader(int tag) => _Der(next(tag));

  /// The reader for the only element of the input, which must have [tag].
  _Der single(int tag) {
    final r = nextReader(tag);
    if (hasMore) throw const FormatException('trailing data');
    return r;
  }

  /// What is left to read.
  Uint8List get rest => Uint8List.sublistView(_b, _pos);
}

Uint8List _hex(String s) => Uint8List.fromList([
      for (var i = 0; i < s.length; i += 2) int.parse(s.substring(i, i + 2), radix: 16),
    ]);

String _hexOf(List<int> b) => b.map((x) => x.toRadixString(16).padLeft(2, '0')).join();

BigInt _bigIntOf(List<int> b) => b.isEmpty ? BigInt.zero : BigInt.parse(_hexOf(b), radix: 16);

Uint8List _leftPad(Uint8List b, int len) =>
    b.length >= len ? b : (Uint8List(len)..setRange(len - b.length, len, b));

bool _sameBytes(List<int> a, List<int> b) {
  if (a.length != b.length) return false;
  for (var i = 0; i < a.length; i++) {
    if (a[i] != b[i]) return false;
  }
  return true;
}
