# Passbubble — Privacy Policy

_Last updated: 22 June 2026_

Passbubble is a self-hosted, end-to-end encrypted password manager. This policy
explains what data the Passbubble app processes, how, and who controls it.

## 1. Who is responsible

Passbubble is **self-hosted software**: you (or whoever operates the Passbubble
server you connect to) are the controller of the data stored in your vault. The
app developer does **not** operate a central service and does **not** receive,
store, or have access to your vault data.

- **App developer / contact:** Gerald Hofbauer — <info@geraldhofbauer.net>
- **Data controller:** the operator of the server instance you choose to connect to.

## 2. What data the app processes

The Passbubble app processes only the data you provide to operate your vault:

- **Account data** — your email address and authentication credentials, used to
  sign in to your chosen server.
- **Vault contents** — passwords, notes, TOTP secrets, and related entries you
  create.
- **Device data** — a locally stored authentication token and your encrypted
  private keys, held in the operating system's secure storage (Keychain on
  Apple platforms).

## 3. End-to-end encryption (zero knowledge)

All vault contents are **encrypted on your device before** they are sent to the
server. The server only ever stores and serves **encrypted blobs** and
per-recipient wrapped data keys.

- **Key exchange:** hybrid X25519 (classical) + ML-KEM-768 (post-quantum).
- **Symmetric encryption:** AES-256-GCM for entry payloads and for encrypting
  your private keys at rest.
- **Key derivation:** Argon2id derives your master key from your master
  password. **Your master password is never transmitted to the server.**

Because of this design, neither the server operator nor the app developer can
read your stored passwords.

## 4. Biometric authentication

If you enable Face ID / Touch ID, biometric verification happens entirely on
your device through Apple's local authentication framework. Passbubble never
receives or stores your biometric data.

## 5. Data sharing

Passbubble does **not** sell or share your data with third parties. There is no
third-party analytics, advertising, or tracking SDK in the app. Network
communication occurs **only** between the app and the server instance you
configure.

## 6. Data retention and deletion

Your vault data resides on the server instance you connect to and is retained
according to that operator's configuration. You can delete individual entries or
your entire account from within the app; deletion removes the corresponding
encrypted records from the server.

## 7. Children's privacy

Passbubble is not directed at children under 13 and does not knowingly collect
data from them.

## 8. Changes to this policy

We may update this policy from time to time. Material changes will be reflected
by updating the "Last updated" date above.

## 9. Contact

Questions about this policy: <info@geraldhofbauer.net>
