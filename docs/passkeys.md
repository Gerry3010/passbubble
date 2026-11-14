# Passkeys

Passbubble stores passkeys (WebAuthn credentials) in the vault and acts as the
authenticator for them. This file is the contract every client follows: the
browser extension, the iOS AutoFill extension, and later Android and macOS. The
server never sees a passkey; it only stores the encrypted entry data.

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
      "created_at": "2026-11-14T03:00:00Z"
    }
  ]
}
```

| Field | Meaning |
|---|---|
| `credential_id` | base64url without padding. 16 random bytes for passkeys we create; imported ones keep their own id. |
| `rp_id` | The relying party id, e.g. `example.com`. Matching uses this field, not the entry URL. |
| `rp_name` | Display name the site sent. Optional. |
| `user_handle` | The site's `user.id`, base64url without padding. Returned with every assertion. |
| `user_name`, `user_display_name` | What the site sent as `user.name` and `user.displayName`. The display name is optional. |
| `key_algorithm` | COSE algorithm. Only `-7` (ES256, P-256) for now. |
| `private_key` | PKCS#8 DER of the P-256 private key, base64url without padding. |
| `sign_count` | Always `0`. See below. |
| `discoverable` | `true` for every passkey we create. |
| `created_at` | RFC 3339, UTC. |

Rules for writers:

- A client that does not handle passkeys must keep the `passkeys` key unchanged
  when it saves an entry, like every other key it does not know.
- Unknown keys inside a passkey record are kept as well.
- A password entry may hold a passkey and no password. Its `password` is then
  empty, and autofill must not offer it as a password.

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
Requests from cross-origin iframes are handed to the browser in this version.
On iOS the system builds the client data and passes its hash.

**rp id check (browser extension only):** the request's `rpId` (default: the
page's host) must equal the page's host or be a registrable suffix of it, and
the page must be `https` (or `localhost`). Otherwise the request goes to the
browser unchanged.

**Creating a passkey:**

1. If any id in `excludeCredentials` matches a stored passkey for this `rp_id`,
   fail with `InvalidStateError` (the account already has one).
2. Generate the P-256 key pair and a 16-byte credential id.
3. Pick the entry. Candidates are `password` entries whose URL host is `rp_id`
   or a subdomain of it, and whose username equals `user_name` (ignoring case).
   The user can choose one of them or a new entry. A new entry gets
   `name = rp_name` (or `rp_id`), `url = https://<rp_id>`,
   `username = user_name`.
4. If the chosen entry already holds a passkey with the same `rp_id` and
   `user_handle`, the new record replaces it: the site registered the same
   account again.

**Signing in:** candidates are all stored passkeys whose `rp_id` equals the
request's `rpId`. A non-empty `allowCredentials` narrows them to the listed
ids. The assertion returns `user_handle` as `userHandle`. With no candidates,
the browser or OS takes over.

## iOS AutoFill extension

The extension cannot decrypt the vault (hybrid KEM), so the app hands it what
it needs through the App Group keychain (`group.net.geraldhofbauer.passbubble`),
as it already does for passwords and TOTP. Channel:
`net.geraldhofbauer.passbubble/autofill`.

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

## Not yet covered

- Android Credential Manager provider.
- macOS AutoFill credential provider (passkeys in Safari and Chrome on a Mac).
- Import and export of passkeys (Bitwarden JSON `fido2Credentials`, the FIDO
  Credential Exchange Format).
