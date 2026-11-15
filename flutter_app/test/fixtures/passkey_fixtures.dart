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

// One fixed P-256 test key (made with `openssl ecparam -name prime256v1
// -genkey -noout | openssl pkcs8 -topk8 -nocrypt`) in the forms a passkey
// import meets, and keys a passkey must refuse. Test data only.

/// The key in the long PKCS#8 form with the public key (138 bytes), as
/// base64url without padding: what Passbubble stores.
const p256Long =
    'MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQge01EP_fT8nvMZwuu40bdp0k6k6zI9ZtDRpSHrpVaHGyh'
    'RANCAARrLn_U7MNVddnsEpvpK3ADEkewJ6HQ2vP1-CnkeVS-r4jnRtLQbVLgZtbgt2Ib32-6XVIVLhAjuovVFbRo6Pxv';

/// The same key as a PKCS#8 PEM block.
const p256Pkcs8Pem = '''-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQge01EP/fT8nvMZwuu
40bdp0k6k6zI9ZtDRpSHrpVaHGyhRANCAARrLn/U7MNVddnsEpvpK3ADEkewJ6HQ
2vP1+CnkeVS+r4jnRtLQbVLgZtbgt2Ib32+6XVIVLhAjuovVFbRo6Pxv
-----END PRIVATE KEY-----
''';

/// The same key as a SEC1 PEM block.
const p256Sec1Pem = '''-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIHtNRD/30/J7zGcLruNG3adJOpOsyPWbQ0aUh66VWhxsoAoGCCqGSM49
AwEHoUQDQgAEay5/1OzDVXXZ7BKb6StwAxJHsCeh0Nrz9fgp5HlUvq+I50bS0G1S
4GbW4LdiG99vul1SFS4QI7qL1RW0aOj8bw==
-----END EC PRIVATE KEY-----
''';

/// The same key as short PKCS#8 (no curve in the inner key, no public key),
/// standard base64 with padding.
const p256Short =
    'MIFBAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBCcwJQIBAQQge01EP/fT8nvMZwuu40bdp0k6k6zI9ZtDRpSHrpVaHGw=';

/// The same key as PKCS#8 whose inner key names the curve but holds no public
/// key, standard base64.
const p256WithParams =
    'ME0CAQAwEwYHKoZIzj0CAQYIKoZIzj0DAQcEMzAxAgEBBCB7TUQ/99Pye8xnC67jRt2nSTqTrMj1m0NGlIeulVocbKAKBggqhkjOPQMBBw==';

/// A second P-256 key, SEC1 PEM after an EC PARAMETERS block (`openssl ecparam
/// -genkey` without `-noout`).
const p256OtherWithParamsPem = '''-----BEGIN EC PARAMETERS-----
BggqhkjOPQMBBw==
-----END EC PARAMETERS-----
-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIM2lWqUM8knjOOmo0+9nugwHZIBeRcEJ+7ZXU+nGL2xjoAoGCCqGSM49
AwEHoUQDQgAEjJwThCrjOrEnSfaV9w4uuKQ1vlHz873nf2df6G8pW//WYEL4MTuz
68RNFkWgesCwrIF8SQt861Rr7ekxGSJIkA==
-----END EC PRIVATE KEY-----
''';

/// A P-384 key (PKCS#8 PEM).
const p384Pem = '''-----BEGIN PRIVATE KEY-----
MIG2AgEAMBAGByqGSM49AgEGBSuBBAAiBIGeMIGbAgEBBDCONleGI1hLoigIrrch
gQEodMdLSAVZN5cM7EeLEWZFbvabd0TAKrv8gbYQV9LcxcKhZANiAAQJ1zEMhDk9
P5kDAZHAFKq9Z+TPwNeqV/SpZghuJYuppbQDj4QTCOb1KDjyrPjAFQqQDSSHh19S
k3LlUObG2nKFJtuxX2szWvNadnredywhDKtFg2/fInyuo+7RNtByqdI=
-----END PRIVATE KEY-----
''';

/// A secp256k1 key (SEC1 PEM).
const secp256k1Pem = '''-----BEGIN EC PRIVATE KEY-----
MHQCAQEEIOsChi4gg8n6DHgcDpWb3Gcnxm76vHBEzwmhU2PhtFc2oAcGBSuBBAAK
oUQDQgAE12H5c2aEpLRWeaAE6GX+byZr20k5FiG3vc+Syl8p/+MNOgrIznuT/nlo
7oc70VLyDYaWm31hr6ALoC+4eivY0w==
-----END EC PRIVATE KEY-----
''';

/// An Ed25519 key (PKCS#8 PEM).
const ed25519Pem = '''-----BEGIN PRIVATE KEY-----
MC4CAQAwBQYDK2VwBCIEIF+0Fo0rKXB9Oi9p/K0D35WnTQuafuNyZwJQuoZFHsWp
-----END PRIVATE KEY-----
''';
