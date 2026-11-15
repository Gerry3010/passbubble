# Passkeys

Passbubble stores passkeys (WebAuthn credentials) in the vault and acts as the
authenticator for them. This file is the contract every client follows: the
browser extension, the iOS and macOS AutoFill extensions, the app and the CLI.
The server never sees a passkey; it only stores the encrypted entry data.

## Storage

A passkey belongs to an entry of type `password`. The decrypted entry data
carries a `passkeys` array next to `username`, `password` and the rest:

```json
{
  "username": "alice@example.com",
  "password": "",
  "passkeys": [
    {
      "credential_id": "q2D0bQ2u3E7XoWlS1yZbVw",
      "rp_id": "example.com",
      "rp_name": "Example",
      "user_handle": "dXNlci0xMjM0",
      "user_name": "alice@example.com",
      "user_display_name": "Alice",
      "key_algorithm": -7,
      "private_key": "MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQg…",
      "sign_count": 0,
      "discoverable": true,
      "prf_secret": "3q2-7wAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
      "created_at": "2026-11-14T03:00:00Z"
    }
  ]
}
```

| Field | Meaning |
|---|---|
| `credential_id` | base64url without padding. 16 random bytes for passkeys we create; imported ones keep their own id (at most 1023 bytes). |
| `rp_id` | The relying party id, e.g. `example.com`. Matching uses this field, not the entry URL. |
| `rp_name` | Display name the site sent. Optional. |
| `user_handle` | The site's `user.id`, base64url without padding. Returned with every assertion. |
| `user_name`, `user_display_name` | What the site sent as `user.name` and `user.displayName`. The display name is optional. |
| `key_algorithm` | COSE algorithm. Only `-7` (ES256, P-256) for now. |
| `private_key` | PKCS#8 DER of the P-256 private key, base64url without padding. Writers store the form that includes the public key (138 bytes, what WebCrypto exports). Readers accept PKCS#8 with or without the optional parameters and public key. |
| `sign_count` | Always `0`. See below. |
| `discoverable` | `true` for every passkey we create. |
| `prf_secret` | Optional. 32 random bytes, base64url, generated when a client creates a passkey. Unused for now; it lets a later version answer the WebAuthn `prf` extension for passkeys created today. Imported passkeys may lack it. |
| `created_at` | RFC 3339, UTC. |

Rules for writers:

- A client that does not handle passkeys must keep the `passkeys` key unchanged
  when it saves an entry, like every other key it does not know.
- Unknown keys inside a passkey record are kept as well.
- A password entry may hold a passkey and no password. Its `password` is then
  empty, autofill must not offer it as a password, and filling a login form
  from it must not overwrite a password the user typed.
- Readers accept `credential_id` and `user_handle` in standard base64 too and
  store them as base64url without padding.
- Version history keeps old entry data, so a deleted or replaced passkey stays
  in the history until the history is purged.

## Authenticator behaviour

These rules hold for every client that creates or uses passkeys.

**AAGUID:** `47683042-7680-4bfb-92d5-9410ed00226d` (Passbubble).

**Algorithm:** ES256 only. If a creation request's `pubKeyCredParams` has no
`-7`, Passbubble declines and the browser or OS takes over.

**Sign counter:** always `0`, and it is never written back. Synced passkeys from
Apple and Google do the same. WebAuthn treats a counter that stays at zero as
"not supported", and no login has to save the entry.

**Authenticator data:**

```
SHA-256(rp_id)            32 bytes
flags                      1 byte
sign count                 4 bytes, big endian, 0
attested credential data   only on create
```

Flags: UP `0x01`, UV `0x04`, BE `0x08`, BS `0x10`, AT `0x40`. An assertion
sends `0x1D`, a creation `0x5D`. UV is set because the vault is unlocked (master
password, PIN or biometrics) and the user confirmed the request in Passbubble's
UI. When a request demands user verification and the client cannot vouch for
it (for example a locked vault), the client asks for it first.

**Attested credential data:** AAGUID (16 bytes), the credential id length
(2 bytes, big endian), the credential id, then the public key as a COSE EC2 key
in canonical CBOR: `{1: 2, 3: -7, -1: 1, -2: x, -3: y}` with 32-byte `x` and `y`.

**Attestation object:** `{"fmt": "none", "attStmt": {}, "authData": <bytes>}`,
canonical CBOR, keys in that order.

**Signature:** ECDSA P-256 with SHA-256 over `authenticatorData ‖ clientDataHash`,
DER encoded. WebCrypto returns the raw `r ‖ s` form, which has to be converted.

**Client data (browser extension only):** the extension builds
`clientDataJSON` itself, with the keys in this order:
`{"type":"webauthn.create"|"webauthn.get","challenge":"<base64url>","origin":"https://…","crossOrigin":false}`.
The extension only answers requests from the top frame, so `crossOrigin` is
always `false` and there is no `topOrigin`. Requests from iframes go to the
browser. On iOS and macOS the system builds the client data and passes its hash.

**Transports:** responses report `["internal", "hybrid"]`. With `internal`
alone, browsers would hide the phone option for these passkeys.

**Extensions:** `credProps` is answered with `{rk: true}` when requested. `prf`
on creation answers `{enabled: false}`, `largeBlob` answers `{supported: false}`,
`appid` answers `false`. Everything else is left out of the results.

**Origin and rp id check (browser extension only):**

- The origin comes from the browser's sender information for the top frame,
  never from the page's message.
- The page must be `https`, or `http` on `localhost`. IP addresses go to the
  browser.
- The rp id is lowercased, without a trailing dot or port. It must equal the
  page's host or be a parent domain of it on a label boundary, and it must not
  be a public suffix (Public Suffix List). `www.example.com` and `example.com`
  are different rp ids.
- Anything else goes to the browser unchanged.

**Creating a passkey:**

1. If any id in `excludeCredentials` matches a stored passkey for this `rp_id`,
   tell the user the passkey is already saved, and fail with
   `InvalidStateError` only after they confirm. Without that consent a page
   could probe which credential ids the vault holds.
2. Generate the P-256 key pair, a 16-byte credential id and the `prf_secret`.
   Passkeys are always created discoverable, whatever `residentKey` asks for.
   Every `attestation` preference gets `fmt: "none"`.
3. Save the entry before answering the site. If saving fails, the site gets
   `NotAllowedError`, so it never registers a credential the vault does not hold.
4. Pick the entry. Candidates are `password` entries whose URL host is `rp_id`
   or a subdomain of it, and whose username equals `user_name` (ignoring case).
   The user can choose one of them or a new entry. A new entry gets
   `name = rp_name` (or `rp_id`), `url = https://<rp_id>`,
   `username = user_name`.
5. If the chosen entry already holds a passkey with the same `rp_id` and
   `user_handle`, the new record replaces it: the site registered the same
   account again.

**Signing in:** candidates are all stored passkeys whose `rp_id` equals the
request's `rpId`. A non-empty `allowCredentials` narrows them to the listed
ids. The assertion returns `user_handle` as `userHandle`. With no candidates,
the browser or OS takes over.

## iOS and macOS AutoFill extensions

The extension cannot decrypt the vault (hybrid KEM), so the app hands it what
it needs through the App Group keychain (`group.net.geraldhofbauer.passbubble`
on iOS; macOS uses its own access group), as it already does for passwords and
TOTP. Channel: `net.geraldhofbauer.passbubble/autofill`. iOS and macOS answer
the same method names, including the existing `iosSyncCredentials` and
`iosClearVault`.

| Keychain item | Written by | Content |
|---|---|---|
| `autofill_passkeys` | app, method `syncPasskeys(json)` | JSON array: every passkey record of the vault plus `entry_id` and `entry_name`. |
| `pending_passkeys` | AutoFill extension | JSON array of passkey records the extension created, not yet in the vault. |

- `syncPasskeys(json)` replaces `autofill_passkeys` and registers an
  `ASPasskeyCredentialIdentity` per record (`relyingPartyIdentifier = rp_id`,
  `userName = user_name`, `credentialID`, `userHandle`,
  `recordIdentifier = entry_id`). Password, one-time-code and passkey
  identities have to end up in the store together; a later replace must not
  drop the others.
- `clearVault` (sign-out) deletes `autofill_passkeys` too. `pending_passkeys`
  stays, since those passkeys exist nowhere else yet.
- A passkey the extension creates goes to `pending_passkeys` and can be used
  for sign-ins straight away; lookups read both items.
- After every unlock the app calls `readPendingPasskeys()` (JSON array, `[]`
  when empty), saves the records into the vault with the entry rules above,
  then calls `clearPendingPasskeys(ids)` with the `credential_id`s it saved.
  The extension removes exactly those, so a passkey created in between is not
  lost.

Method arguments on the channel:

| Method | Direction | Arguments | Returns |
|---|---|---|---|
| `iosSyncCredentials` | app → native | `{"credentials": <JSON string>}` | nothing |
| `iosClearVault` | app → native | none | nothing |
| `syncPasskeys` | app → native | `{"passkeys": <JSON string>}` | nothing |
| `readPendingPasskeys` | app → native | none | JSON string, `"[]"` when empty |
| `clearPendingPasskeys` | app → native | `{"ids": [<credential_id>, …]}`, ids exactly as the extension wrote them | nothing |
| `credentialExchangeReceived` | native → app | the CXF JSON string (a map `{"json": …}` is accepted too) | nothing |

The app ignores `NotImplemented` and errors from every app → native call, so a
platform without the native side simply does nothing.

## Adding a passkey by hand

The app can add a passkey to a `password` entry from its parts: rp id, user
name, user handle, credential id and private key. The key may be a PEM block
(`PRIVATE KEY` or `EC PRIVATE KEY`) or base64 of the DER. The app checks that
it is a P-256 key and stores it in the long PKCS#8 form.

## Bitwarden JSON

`login.fido2Credentials[]` maps to `passkeys[]`, in both directions:

| Bitwarden | Passbubble |
|---|---|
| `credentialId`: a GUID, or `b64.<base64url>` | `credential_id`: the GUID's 16 bytes, or the decoded bytes. Export always writes `b64.<base64url>`. |
| `keyType` `public-key`, `keyAlgorithm` `ECDSA`, `keyCurve` `P-256` | `key_algorithm: -7`. Other values are skipped and counted. |
| `keyValue` (base64url PKCS#8) | `private_key`, normalised |
| `rpId`, `rpName`, `userName`, `userDisplayName`, `userHandle` | `rp_id`, `rp_name`, `user_name`, `user_display_name`, `user_handle` |
| `counter` (string) | dropped on import, `"0"` on export |
| `discoverable` (`"true"`/`"false"`) | `discoverable` |
| `creationDate` | `created_at` |

A merging import combines passkeys by `credential_id` instead of replacing
the array.

## Import from Apple Passwords (Credential Exchange)

On iOS and macOS 26 the native side receives exported accounts through
`ASCredentialImportManager` and hands them to the app as JSON in the FIDO
Credential Exchange Format (CXF), via the channel method
`credentialExchangeReceived(json)` (native calls Dart). The app parses the CXF
items (`basic-auth`, `passkey`, `totp`, `note`, `credit-card`; others are
counted and reported), shows the usual import screen and runs them through the
regular import with duplicate handling. While the vault is locked, the data
waits in memory until the next unlock. A CXF JSON file can also be imported
from the import screen directly.

## Not yet covered

- Android Credential Manager provider.
- Answering the `prf` extension (the `prf_secret` is already stored).
